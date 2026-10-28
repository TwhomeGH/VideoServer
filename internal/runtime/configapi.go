package runtime

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"
)

var editableConfigFields = []string{
	"host", "preferredPort", "fallbackPorts", "rememberLastSuccessfulPort",
	"autoScan", "scanIntervalSeconds", "scanIdleMaxSeconds", "fileStableSeconds",
	"autoBackfill", "backfillIntervalSeconds", "backfillDailyLimit", "backfillDailyGiB", "hashLimitMiBps", "autoCover", "coverIntervalSeconds",
}

// ConfigState 保存啟動時載入的基準設定、目前生效值與管理頁覆寫。
// 儲存只寫入資料目錄的覆寫檔；真正套用需要重新啟動程序。
type ConfigState struct {
	mu           sync.RWMutex
	base         Config
	running      Config
	effective    Config
	overrides    Override
	source       string
	overridePath string
	network      *NetworkRuntime
	restart      func()
	restartOnce  sync.Once
	publisher    interface{ Publish(string) }
}

func (c *ConfigState) SetPublisher(publisher interface{ Publish(string) }) {
	c.publisher = publisher
}

func (c *ConfigState) publishChanged() {
	if c.publisher != nil {
		c.publisher.Publish("config")
	}
}

// NewConfigState 建立設定 API 狀態；running 是目前程序使用的完整設定。
func NewConfigState(base, running Config, overrides Override, source, overridePath string, network *NetworkRuntime, restart func()) *ConfigState {
	return &ConfigState{
		base: base, running: running, effective: running, overrides: overrides,
		source: source, overridePath: overridePath, network: network, restart: restart,
	}
}

type configView struct {
	Source          string          `json:"source"`
	OverridePath    string          `json:"overridePath"`
	Base            Config          `json:"base"`
	Effective       Config          `json:"effective"`
	Overrides       Override        `json:"overrides"`
	Editable        []string        `json:"editable"`
	RestartRequired bool            `json:"restartRequired"`
	Network         *NetworkRuntime `json:"network,omitempty"`
}

type configInput struct {
	Host                       *string `json:"host"`
	PreferredPort              *int    `json:"preferredPort"`
	FallbackPorts              *[]int  `json:"fallbackPorts"`
	AutoScan                   *bool   `json:"autoScan"`
	ScanIntervalSeconds        *int    `json:"scanIntervalSeconds"`
	ScanIdleMaxSeconds         *int    `json:"scanIdleMaxSeconds"`
	FileStableSeconds          *int    `json:"fileStableSeconds"`
	AutoBackfill               *bool   `json:"autoBackfill"`
	AutoCover                  *bool   `json:"autoCover"`
	CoverIntervalSeconds       *int    `json:"coverIntervalSeconds"`
	BackfillIntervalSeconds    *int    `json:"backfillIntervalSeconds"`
	BackfillDailyLimit         *int    `json:"backfillDailyLimit"`
	BackfillDailyGiB           *int    `json:"backfillDailyGiB"`
	HashLimitMiBps             *int    `json:"hashLimitMiBps"`
	RememberLastSuccessfulPort *bool   `json:"rememberLastSuccessfulPort"`
}

// Register 加入設定讀寫與重啟 API。
func (c *ConfigState) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/config", func(w http.ResponseWriter, r *http.Request) {
		writeConfigJSON(w, http.StatusOK, c.view())
	})
	mux.HandleFunc("PUT /api/v1/config", c.handleUpdate)
	mux.HandleFunc("DELETE /api/v1/config/overrides", c.handleReset)
	mux.HandleFunc("POST /api/v1/system/restart", c.handleRestart)
}

// view 建立一致的設定快照；呼叫端不需要自行加鎖。
func (c *ConfigState) view() configView {
	c.mu.RLock()
	defer c.mu.RUnlock()
	network := c.network
	if network != nil {
		copy := *network
		network = &copy
	}
	return configView{
		Source: c.source, OverridePath: c.overridePath, Base: c.base, Effective: c.effective,
		Overrides: c.overrides, Editable: append([]string(nil), editableConfigFields...),
		RestartRequired: !reflect.DeepEqual(c.effective, c.running), Network: network,
	}
}

func (c *ConfigState) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var input configInput
	if !decodeConfig(w, r, &input) {
		return
	}
	next, err := c.applyInput(input)
	if err != nil {
		writeConfigJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := next.Validate(); err != nil {
		writeConfigJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	override := BuildOverride(c.base, next)
	if err := SaveOverride(c.overridePath, override); err != nil {
		writeConfigJSON(w, http.StatusInternalServerError, map[string]string{"error": "無法保存設定覆寫：" + err.Error()})
		return
	}
	c.mu.Lock()
	c.overrides = override
	c.effective = override.Apply(c.base)
	c.mu.Unlock()
	c.publishChanged()
	writeConfigJSON(w, http.StatusOK, c.view())
}

func (c *ConfigState) applyInput(input configInput) (Config, error) {
	if input.Host == nil || input.PreferredPort == nil || input.FallbackPorts == nil || input.AutoScan == nil ||
		input.ScanIntervalSeconds == nil || input.ScanIdleMaxSeconds == nil || input.FileStableSeconds == nil || input.RememberLastSuccessfulPort == nil ||
		input.AutoBackfill == nil || input.BackfillIntervalSeconds == nil || input.BackfillDailyLimit == nil || input.BackfillDailyGiB == nil || input.HashLimitMiBps == nil || input.AutoCover == nil || input.CoverIntervalSeconds == nil {
		return Config{}, errMissingConfigField
	}
	next := c.base
	next.Host = strings.TrimSpace(*input.Host)
	next.PreferredPort = *input.PreferredPort
	next.FallbackPorts = append([]int(nil), (*input.FallbackPorts)...)
	next.AutoScan = *input.AutoScan
	next.ScanIntervalSeconds = *input.ScanIntervalSeconds
	next.ScanIdleMaxSeconds = *input.ScanIdleMaxSeconds
	next.FileStableSeconds = *input.FileStableSeconds
	next.AutoBackfill = *input.AutoBackfill
	next.BackfillIntervalSeconds = *input.BackfillIntervalSeconds
	next.BackfillDailyLimit = *input.BackfillDailyLimit
	next.BackfillDailyGiB = *input.BackfillDailyGiB
	next.HashLimitMiBps = *input.HashLimitMiBps
	next.AutoCover = *input.AutoCover
	next.CoverIntervalSeconds = *input.CoverIntervalSeconds
	next.RememberLastSuccessfulPort = *input.RememberLastSuccessfulPort
	if len(next.FallbackPorts) == 0 {
		next.RememberLastSuccessfulPort = false
	}
	return next, nil
}

var errMissingConfigField = errors.New("設定表單缺少欄位")

func (c *ConfigState) handleReset(w http.ResponseWriter, r *http.Request) {
	if !sameConfigOrigin(r) {
		writeConfigJSON(w, http.StatusForbidden, map[string]string{"error": "不允許跨站管理請求"})
		return
	}
	if err := SaveOverride(c.overridePath, Override{}); err != nil {
		writeConfigJSON(w, http.StatusInternalServerError, map[string]string{"error": "無法移除設定覆寫：" + err.Error()})
		return
	}
	c.mu.Lock()
	c.overrides = Override{}
	c.effective = c.base
	c.mu.Unlock()
	c.publishChanged()
	writeConfigJSON(w, http.StatusOK, c.view())
}

func (c *ConfigState) handleRestart(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Confirm bool `json:"confirm"`
	}
	if !decodeConfig(w, r, &request) {
		return
	}
	if !request.Confirm {
		writeConfigJSON(w, http.StatusBadRequest, map[string]string{"error": "請確認重新啟動"})
		return
	}
	if c.restart == nil {
		writeConfigJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "此執行環境不支援重新啟動"})
		return
	}
	c.publishChanged()
	writeConfigJSON(w, http.StatusAccepted, map[string]string{"status": "restarting"})
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	c.restartOnce.Do(func() {
		go func() {
			time.Sleep(150 * time.Millisecond)
			c.restart()
		}()
	})
}

// decodeConfig 檢查來源、型別與大小；與其他管理 API 的 JSON 規則一致。
func decodeConfig(w http.ResponseWriter, r *http.Request, value any) bool {
	if !sameConfigOrigin(r) {
		writeConfigJSON(w, http.StatusForbidden, map[string]string{"error": "不允許跨站管理請求"})
		return false
	}
	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if contentType != "application/json" {
		writeConfigJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "請使用 application/json"})
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		writeConfigJSON(w, http.StatusBadRequest, map[string]string{"error": "請求 JSON 格式無效"})
		return false
	}
	if decoder.Decode(new(any)) != io.EOF {
		writeConfigJSON(w, http.StatusBadRequest, map[string]string{"error": "請求必須只包含一個 JSON 物件"})
		return false
	}
	return true
}

func writeConfigJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

// sameConfigOrigin 拒絕瀏覽器跨站請求；沒有 Origin 的命令列用戶端不受影響。
func sameConfigOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host == r.Host
}
