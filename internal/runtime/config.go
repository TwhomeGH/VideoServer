// Package runtime 提供三個服務共用的啟動與診斷基礎。
package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

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
	ScanIdleMaxSeconds         int    `json:"scanIdleMaxSeconds"`
	FileStableSeconds          int    `json:"fileStableSeconds"`
	AutoBackfill               bool   `json:"autoBackfill"`
	BackfillIntervalSeconds    int    `json:"backfillIntervalSeconds"`
	BackfillDailyLimit         int    `json:"backfillDailyLimit"`
	BackfillDailyGiB           int    `json:"backfillDailyGiB"`
	HashLimitMiBps             int    `json:"hashLimitMiBps"`
	RememberLastSuccessfulPort bool   `json:"rememberLastSuccessfulPort"`
	FFprobePath                string `json:"ffprobePath"`
	ProbeTimeoutSeconds        int    `json:"probeTimeoutSeconds"`
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
	c := Config{Host: "0.0.0.0", PreferredPort: port, FallbackPorts: []int{port + 1, port + 2, port + 3}, WebDirectory: "web/admin/dist", DataDirectory: dataDirectory, AutoScan: true, ScanIntervalSeconds: 30, ScanIdleMaxSeconds: 21600, FileStableSeconds: 10, AutoBackfill: false, BackfillIntervalSeconds: 60, BackfillDailyLimit: 50, BackfillDailyGiB: 20, HashLimitMiBps: 64, RememberLastSuccessfulPort: true, FFprobePath: "ffprobe", ProbeTimeoutSeconds: 60}
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

// Override 是管理頁可調整的設定；指標為 nil 表示沿用基準設定。
// dataDirectory 與 webDirectory 不在這裡，避免執行中改變資料或靜態檔位置。
type Override struct {
	Host                       *string `json:"host,omitempty"`
	PreferredPort              *int    `json:"preferredPort,omitempty"`
	FallbackPorts              *[]int  `json:"fallbackPorts,omitempty"`
	AutoScan                   *bool   `json:"autoScan,omitempty"`
	ScanIntervalSeconds        *int    `json:"scanIntervalSeconds,omitempty"`
	ScanIdleMaxSeconds         *int    `json:"scanIdleMaxSeconds,omitempty"`
	FileStableSeconds          *int    `json:"fileStableSeconds,omitempty"`
	AutoBackfill               *bool   `json:"autoBackfill,omitempty"`
	BackfillIntervalSeconds    *int    `json:"backfillIntervalSeconds,omitempty"`
	BackfillDailyLimit         *int    `json:"backfillDailyLimit,omitempty"`
	BackfillDailyGiB           *int    `json:"backfillDailyGiB,omitempty"`
	HashLimitMiBps             *int    `json:"hashLimitMiBps,omitempty"`
	RememberLastSuccessfulPort *bool   `json:"rememberLastSuccessfulPort,omitempty"`
}

// LoadOverride 讀取 dataDirectory 的管理頁覆寫檔；不存在時回傳空覆寫。
// 未知欄位會讓載入失敗，避免舊版或手動編輯的錯誤欄位被悄悄忽略。
func LoadOverride(path string) (Override, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Override{}, nil
	}
	if err != nil {
		return Override{}, err
	}
	var value Override
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return Override{}, fmt.Errorf("設定覆寫檔格式無效：%w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return Override{}, fmt.Errorf("設定覆寫檔必須只包含一個 JSON 物件")
	}
	return value, nil
}

// Apply 將覆寫套用到基準設定；未覆寫的欄位保留基準值。
func (o Override) Apply(base Config) Config {
	value := base
	if o.Host != nil {
		value.Host = *o.Host
	}
	if o.PreferredPort != nil {
		value.PreferredPort = *o.PreferredPort
	}
	if o.FallbackPorts != nil {
		value.FallbackPorts = append([]int(nil), (*o.FallbackPorts)...)
	}
	if o.AutoScan != nil {
		value.AutoScan = *o.AutoScan
	}
	if o.ScanIntervalSeconds != nil {
		value.ScanIntervalSeconds = *o.ScanIntervalSeconds
	}
	if o.ScanIdleMaxSeconds != nil {
		value.ScanIdleMaxSeconds = *o.ScanIdleMaxSeconds
	}
	if o.FileStableSeconds != nil {
		value.FileStableSeconds = *o.FileStableSeconds
	}
	if o.AutoBackfill != nil {
		value.AutoBackfill = *o.AutoBackfill
	}
	if o.BackfillIntervalSeconds != nil {
		value.BackfillIntervalSeconds = *o.BackfillIntervalSeconds
	}
	if o.BackfillDailyLimit != nil {
		value.BackfillDailyLimit = *o.BackfillDailyLimit
	}
	if o.BackfillDailyGiB != nil {
		value.BackfillDailyGiB = *o.BackfillDailyGiB
	}
	if o.HashLimitMiBps != nil {
		value.HashLimitMiBps = *o.HashLimitMiBps
	}
	if o.RememberLastSuccessfulPort != nil {
		value.RememberLastSuccessfulPort = *o.RememberLastSuccessfulPort
	}
	return value
}

// Empty 回傳覆寫是否沒有任何欄位。
func (o Override) Empty() bool {
	return o.Host == nil && o.PreferredPort == nil && o.FallbackPorts == nil && o.AutoScan == nil &&
		o.ScanIntervalSeconds == nil && o.ScanIdleMaxSeconds == nil && o.FileStableSeconds == nil && o.AutoBackfill == nil &&
		o.BackfillIntervalSeconds == nil && o.BackfillDailyLimit == nil && o.BackfillDailyGiB == nil && o.HashLimitMiBps == nil && o.RememberLastSuccessfulPort == nil
}

// BuildOverride 比較基準與新設定，只保存有差異的可編輯欄位。
// 欄位回到基準值時不會留在覆寫檔，日後修改基準檔仍能生效。
func BuildOverride(base, next Config) Override {
	value := Override{}
	if next.Host != base.Host {
		value.Host = &next.Host
	}
	if next.PreferredPort != base.PreferredPort {
		value.PreferredPort = &next.PreferredPort
	}
	if !slices.Equal(next.FallbackPorts, base.FallbackPorts) {
		ports := append([]int(nil), next.FallbackPorts...)
		value.FallbackPorts = &ports
	}
	if next.AutoScan != base.AutoScan {
		value.AutoScan = &next.AutoScan
	}
	if next.ScanIntervalSeconds != base.ScanIntervalSeconds {
		value.ScanIntervalSeconds = &next.ScanIntervalSeconds
	}
	if next.ScanIdleMaxSeconds != base.ScanIdleMaxSeconds {
		value.ScanIdleMaxSeconds = &next.ScanIdleMaxSeconds
	}
	if next.FileStableSeconds != base.FileStableSeconds {
		value.FileStableSeconds = &next.FileStableSeconds
	}
	if next.AutoBackfill != base.AutoBackfill {
		value.AutoBackfill = &next.AutoBackfill
	}
	if next.BackfillIntervalSeconds != base.BackfillIntervalSeconds {
		value.BackfillIntervalSeconds = &next.BackfillIntervalSeconds
	}
	if next.BackfillDailyLimit != base.BackfillDailyLimit {
		value.BackfillDailyLimit = &next.BackfillDailyLimit
	}
	if next.BackfillDailyGiB != base.BackfillDailyGiB {
		value.BackfillDailyGiB = &next.BackfillDailyGiB
	}
	if next.HashLimitMiBps != base.HashLimitMiBps {
		value.HashLimitMiBps = &next.HashLimitMiBps
	}
	if next.RememberLastSuccessfulPort != base.RememberLastSuccessfulPort {
		value.RememberLastSuccessfulPort = &next.RememberLastSuccessfulPort
	}
	return value
}

// SaveOverride 驗證後原子保存覆寫；空覆寫會移除檔案。
func SaveOverride(path string, value Override) error {
	if value.Empty() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return atomicfile.WriteJSON(path, value)
}

// Validate 檢查完整生效設定，回傳欄位錯誤；不修改設定或寫入磁碟。
func (c Config) Validate() error {
	if c.FFprobePath == "" || c.ProbeTimeoutSeconds < 1 || c.ProbeTimeoutSeconds > 3600 {
		return fmt.Errorf("ffprobePath 不可為空，probeTimeoutSeconds 須介於 1 到 3600")
	}
	if c.DataDirectory == "" {
		return fmt.Errorf("dataDirectory 不可為空")
	}
	if c.Host == "" {
		return fmt.Errorf("host 不可為空")
	}
	if c.ScanIntervalSeconds < 1 || c.ScanIntervalSeconds > 86400 || c.FileStableSeconds < 1 || c.FileStableSeconds > 86400 {
		return fmt.Errorf("掃描間隔與檔案穩定秒數須介於 1 到 86400")
	}
	if c.ScanIdleMaxSeconds < 1800 || c.ScanIdleMaxSeconds > 86400 {
		return fmt.Errorf("掃描閒置上限必須介於 10 到 86400")
	}
	if c.BackfillIntervalSeconds < 10 || c.BackfillIntervalSeconds > 86400 {
		return fmt.Errorf("自動補齊檢查間隔必須介於 10 到 86400")
	}
	if c.BackfillDailyLimit < 0 || c.BackfillDailyLimit > 100000 {
		return fmt.Errorf("每日自動補齊部數必須介於 0 到 100000")
	}
	if c.BackfillDailyGiB < 0 || c.BackfillDailyGiB > 102400 {
		return fmt.Errorf("每日自動補齊容量必須介於 0 到 102400 GiB")
	}
	if c.HashLimitMiBps < 0 || c.HashLimitMiBps > 102400 {
		return fmt.Errorf("SHA-256 讀取上限必須介於 0 到 102400 MiB/s")
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
