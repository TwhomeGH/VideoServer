// Package runtime 提供三個服務共用的啟動與診斷基礎。
package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/TwhomeGH/VideoServer/internal/atomicfile"
)

// Config 定義本機監聽與靜態管理頁面的位置。
// 初版採 JSON，避免尚未使用的設定格式引入外部相依。
type Config struct {
	Host                       string `json:"host"`
	PreferredPort              int    `json:"preferredPort"`
	FallbackPorts              []int  `json:"fallbackPorts"`
	WebDirectory               string `json:"webDirectory"`
	DataDirectory              string `json:"dataDirectory"`
	AutoScan                   bool   `json:"autoScan"`
	ScanIntervalSeconds        int    `json:"scanIntervalSeconds"`
	FileStableSeconds          int    `json:"fileStableSeconds"`
	RememberLastSuccessfulPort bool   `json:"rememberLastSuccessfulPort"`
}

// LoadConfig 讀取 path 的設定；空路徑回傳指定 port、所有 IPv4 介面的預設值。
// service 可指定三服務之一，使未指定 dataDirectory 時各用獨立資料目錄；舊 Node 資料沿用。
// 無效格式或端口會回傳錯誤，避免服務以非預期設定啟動。
func LoadConfig(path string, port int, service ...string) (Config, error) {
	dataDirectory := "data"
	if len(service) > 1 {
		return Config{}, fmt.Errorf("只能指定一個服務")
	}
	if len(service) == 1 {
		name := service[0]
		if name != "video-node" && name != "central" && name != "transcode-worker" {
			return Config{}, fmt.Errorf("未知服務：%s", name)
		}
		dataDirectory = filepath.Join("data", name)
		if name == "video-node" {
			_, err := os.Stat(filepath.Join("data", "library.sqlite"))
			if err == nil {
				dataDirectory = "data"
			} else if !os.IsNotExist(err) {
				return Config{}, err
			}
		}
	}
	c := Config{Host: "0.0.0.0", PreferredPort: port, FallbackPorts: []int{port + 1, port + 2, port + 3}, WebDirectory: "web/admin/dist", DataDirectory: dataDirectory, AutoScan: true, ScanIntervalSeconds: 30, FileStableSeconds: 10, RememberLastSuccessfulPort: true}
	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return c, err
		}
		defer file.Close()
		decoder := json.NewDecoder(file)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&c); err != nil {
			return c, err
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return c, fmt.Errorf("設定檔必須只包含一個 JSON 物件")
		}
	}
	if len(c.FallbackPorts) == 0 {
		c.RememberLastSuccessfulPort = false
	}
	return c, c.Validate()
}

// Validate 檢查完整生效設定，回傳欄位錯誤；不修改設定或寫入磁碟。
func (c Config) Validate() error {
	if c.DataDirectory == "" {
		return fmt.Errorf("dataDirectory 不可為空")
	}
	if c.ScanIntervalSeconds < 1 || c.ScanIntervalSeconds > 86400 || c.FileStableSeconds < 1 || c.FileStableSeconds > 86400 {
		return fmt.Errorf("掃描間隔與檔案穩定秒數須介於 1 到 86400")
	}
	for _, p := range append([]int{c.PreferredPort}, c.FallbackPorts...) {
		if p < 1 || p > 65535 {
			return fmt.Errorf("無效端口：%d", p)
		}
	}
	return nil
}

// SaveConfig 驗證完整設定後原子保存至 path，不變更程序目前的生效值。
// 父目錄須已存在；唯讀掛載或替換前寫入失敗時保留原檔並回報錯誤。
func SaveConfig(path string, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	return atomicfile.WriteJSON(path, config)
}
