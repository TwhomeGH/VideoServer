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
	"strconv"
	"syscall"
	"time"

	"github.com/TwhomeGH/VideoServer/internal/library"
	"github.com/TwhomeGH/VideoServer/pkg/buildinfo"
)

// Listen 依設定順序綁定實際端口；全部失敗時回傳所有監聽錯誤。
func Listen(c Config) (net.Listener, error) {
	var failures []error
	for _, port := range append([]int{c.PreferredPort}, c.FallbackPorts...) {
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
	config, err := LoadConfig(*path, port)
	if err != nil {
		logger.Error("讀取設定失敗", "error", err)
		return 1
	}
	listener, err := Listen(config)
	if err != nil {
		logger.Error("所有端口皆無法監聽", "error", err)
		return 1
	}
	identity := buildinfo.Current(service)
	logger = logger.With("build", identity)
	defer listener.Close()
	var extensions []func(*http.ServeMux)
	if service == "video-node" {
		store, err := library.Open(config.DataDirectory)
		if err != nil {
			logger.Error("開啟影片索引失敗", "error", err)
			return 1
		}
		defer store.Close()
		extensions = append(extensions, store.Register)
	}
	server := &http.Server{Handler: Handler(identity, listener.Addr().String(), config.WebDirectory, extensions...), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			logger.Error("關閉失敗", "error", err)
			server.Close()
		}
	}()
	logger.Info("服務啟動", "listenAddress", listener.Addr().String())
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("服務失敗", "error", err)
		return 1
	}
	return 0
}
