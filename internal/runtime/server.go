package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/TwhomeGH/VideoServer/internal/auth"
	"github.com/TwhomeGH/VideoServer/internal/diagnostics"
	"github.com/TwhomeGH/VideoServer/internal/events"
	"github.com/TwhomeGH/VideoServer/internal/library"
	"github.com/TwhomeGH/VideoServer/internal/media"
	"github.com/TwhomeGH/VideoServer/pkg/buildinfo"
	"github.com/TwhomeGH/VideoServer/pkg/protocol"
)

// Listen 依設定順序綁定實際端口；全部失敗時回傳所有監聽錯誤。
func Listen(c Config) (net.Listener, error) {
	return listenPorts(c, candidatePorts(c, RuntimeState{}))
}

func listenPorts(c Config, ports []int) (net.Listener, error) {
	var failures []error
	for _, port := range ports {
		listener, err := listenTCP(net.JoinHostPort(c.Host, strconv.Itoa(port)))
		if err == nil {
			return listener, nil
		}
		failures = append(failures, err)
	}
	return nil, errors.Join(failures...)
}

// Handler 建立服務診斷與靜態資源路由，extensions 用於註冊服務專屬 API。
// address 是實際監聽位址，非公開 Endpoint；webDirectory 指向已打包管理頁面。
func Handler(identity buildinfo.Identity, address, webDirectory string, extensions ...func(*http.ServeMux)) http.Handler {
	mux := http.NewServeMux()
	protocol.Register(mux, identity.Service)
	for _, register := range extensions {
		register(mux)
	}
	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(struct {
			Status        string             `json:"status"`
			Build         buildinfo.Identity `json:"build"`
			ListenAddress string             `json:"listenAddress"`
		}{"ok", identity, address})
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok"}`)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	mux.Handle("/", http.FileServer(http.Dir(webDirectory)))
	return mux
}

// Main 啟動指定服務與預設端口，回傳程序退出碼；支援中斷後優雅關閉。
func Main(service string, port int) int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	path := flag.String("config", "", "JSON 設定檔路徑")
	flag.Parse()
	baseConfig, err := LoadConfig(*path, port, service)
	if err != nil {
		logger.Error("讀取設定失敗", "error", err)
		return 1
	}
	source := "內建預設值"
	if *path != "" {
		source, err = filepath.Abs(*path)
		if err != nil {
			logger.Error("設定路徑無效", "error", err)
			return 1
		}
	}
	baseConfig.DataDirectory, err = filepath.Abs(baseConfig.DataDirectory)
	if err != nil {
		logger.Error("資料路徑無效", "error", err)
		return 1
	}
	baseConfig.WebDirectory, err = filepath.Abs(baseConfig.WebDirectory)
	if err != nil {
		logger.Error("網頁路徑無效", "error", err)
		return 1
	}
	if err := os.MkdirAll(baseConfig.DataDirectory, 0700); err != nil {
		logger.Error("建立資料目錄失敗", "error", err)
		return 1
	}
	overridePath := filepath.Join(baseConfig.DataDirectory, "config.override.json")
	overrides, err := LoadOverride(overridePath)
	if err != nil {
		logger.Error("讀取設定覆寫失敗", "error", err)
		return 1
	}
	config := overrides.Apply(baseConfig)
	if err := config.Validate(); err != nil {
		logger.Error("設定覆寫無效", "error", err)
		return 1
	}
	authManager, initialPassword, err := auth.Open(baseConfig.DataDirectory, os.Getenv("DVN_ADMIN_PASSWORD"))
	if err != nil {
		logger.Error("初始化管理授權失敗", "error", err)
		return 1
	}
	if initialPassword != "" {
		logger.Warn("已建立管理員密碼，登入後請立即變更；此密碼不會再次顯示",
			"password", initialPassword, "authFile", filepath.Join(baseConfig.DataDirectory, "auth.json"))
	}
	network := NetworkRuntime{StateFile: filepath.Join(config.DataDirectory, "runtime.json"), RememberEnabled: config.RememberLastSuccessfulPort}
	state, stateErr := LoadRuntimeState(network.StateFile, service)
	if stateErr != nil {
		network.Warning = "無法讀取舊狀態，本次沿用設定端口並保留原檔：" + stateErr.Error()
		logger.Warn(network.Warning, "stateFile", network.StateFile)
	}
	network.PreviousPort = state.LastSuccessfulPort
	listener, err := listenPorts(config, candidatePorts(config, state))
	if err != nil {
		logger.Error("所有端口皆無法監聽", "error", err)
		return 1
	}
	identity := buildinfo.Current(service)
	logger = logger.With("build", identity)
	defer listener.Close()
	network.ActivePort = listener.Addr().(*net.TCPAddr).Port
	var restartRequested atomic.Bool
	restartCh := make(chan struct{}, 1)
	requestRestart := func() {
		select {
		case restartCh <- struct{}{}:
		default:
		}
	}
	hub := events.NewHub(512)
	configState := NewConfigState(baseConfig, config, overrides, source, overridePath, &network, requestRestart)
	configState.SetPublisher(hub)
	var extensions []func(*http.ServeMux)
	var scanner *library.Scanner
	var queue *library.ImportQueue
	extensions = append(extensions, authManager.Register)
	extensions = append(extensions, configState.Register)
	if service == "video-node" {
		store, err := library.Open(config.DataDirectory)
		if err != nil {
			logger.Error("開啟影片索引失敗", "error", err)
			return 1
		}
		defer store.Close()
		formats := media.LoadVideoFormats(context.Background(), config.FFprobePath, config.ExtraVideoExtensions)
		for _, warning := range formats.Warnings() {
			logger.Warn("影片格式能力警告", "warning", warning)
		}
		store.SetVideoFormats(formats)
		queue, err = library.NewImportQueue(store, config.FFprobePath, time.Duration(config.ProbeTimeoutSeconds)*time.Second, logger)
		if err != nil {
			logger.Error("初始化匯入佇列失敗", "error", err)
			return 1
		}
		queue.SetHashLimit(config.HashLimitMiBps)
		queue.SetBackfillLimits(config.BackfillDailyLimit, config.BackfillDailyGiB)
		scanner = library.NewScanner(store, config.AutoScan, time.Duration(config.ScanIntervalSeconds)*time.Second, time.Duration(config.FileStableSeconds)*time.Second, logger)
		scanner.SetIdleMax(time.Duration(config.ScanIdleMaxSeconds) * time.Second)
		store.SetPublisher(hub)
		extensions = append(extensions, store.Register)
	}
	if stateErr == nil {
		state = RuntimeState{SchemaVersion: 1, Service: service, Host: config.Host, LastSuccessfulPort: network.ActivePort, UpdatedAt: time.Now().UTC()}
		saveErr := os.MkdirAll(config.DataDirectory, 0700)
		if saveErr == nil {
			saveErr = SaveRuntimeState(network.StateFile, state)
		}
		if saveErr != nil {
			network.Warning = "服務可運作，但無法保存執行狀態：" + saveErr.Error()
			logger.Warn(network.Warning, "stateFile", network.StateFile)
		} else {
			network.Saved = true
		}
	}
	monitor := diagnostics.New(service, func(ctx context.Context) (any, error) {
		if queue == nil {
			return nil, nil
		}
		work, err := queue.WorkStatus(ctx)
		if err != nil {
			return nil, err
		}
		backfill, err := queue.BackfillStatus(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"jobs": work, "scan": scanner.Status(), "backfill": backfill}, nil
	})
	extensions = append(extensions, func(mux *http.ServeMux) { events.Register(mux, hub) })
	extensions = append(extensions, monitor.Register)
	server := &http.Server{Handler: monitor.Middleware(protocol.Middleware(authManager.Middleware(Handler(identity, listener.Addr().String(), config.WebDirectory, extensions...)))), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	monitorDone := make(chan struct{})
	go func() { defer close(monitorDone); monitor.Run(ctx) }()
	defer func() { stop(); <-monitorDone }()
	go func() {
		select {
		case <-restartCh:
			restartRequested.Store(true)
			logger.Info("收到重新啟動請求，準備套用設定")
			stop()
		case <-ctx.Done():
		}
	}()
	if queue != nil {
		queueContext, cancelQueue := context.WithCancel(ctx)
		queueDone := make(chan struct{})
		backfillDone := make(chan struct{})
		go func() { defer close(queueDone); queue.Run(queueContext) }()
		go func() {
			defer close(backfillDone)
			queue.RunBackfill(queueContext, time.Duration(config.BackfillIntervalSeconds)*time.Second, config.AutoBackfill)
		}()
		defer func() { cancelQueue(); <-queueDone; <-backfillDone }()
	}
	if scanner != nil {
		scanContext, cancelScan := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() { defer close(done); scanner.Run(scanContext) }()
		defer func() { cancelScan(); <-done }()
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			logger.Error("關閉失敗", "error", err)
			server.Close()
		}
	}()
	logger.Info("服務啟動", "listenAddress", listener.Addr().String(), "configSource", source, "dataDirectory", config.DataDirectory)
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("服務失敗", "error", err)
		return 1
	}
	if restartRequested.Load() {
		logger.Info("結束目前程序，交由監督程式重新啟動")
		return 75
	}
	return 0
}
