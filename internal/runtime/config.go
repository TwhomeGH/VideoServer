// Package runtime 提供三個服務共用的啟動與診斷基礎。
package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Config 定義本機監聽與靜態管理頁面的位置。
// 初版採 JSON，避免尚未使用的設定格式引入外部相依。
type Config struct {
	Host          string `json:"host"`
	PreferredPort int    `json:"preferredPort"`
	FallbackPorts []int  `json:"fallbackPorts"`
	WebDirectory  string `json:"webDirectory"`
	DataDirectory string `json:"dataDirectory"`
}

// LoadConfig 讀取 path 的設定；空路徑回傳指定 port、所有 IPv4 介面的預設值。
// 無效格式或端口會回傳錯誤，避免服務以非預期設定啟動。
func LoadConfig(path string, port int) (Config, error) {
	c := Config{Host: "0.0.0.0", PreferredPort: port, FallbackPorts: []int{port + 1, port + 2, port + 3}, WebDirectory: "web/admin/dist", DataDirectory: "data"}
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
	if c.DataDirectory == "" {
		return c, fmt.Errorf("dataDirectory 不可為空")
	}
	for _, p := range append([]int{c.PreferredPort}, c.FallbackPorts...) {
		if p < 1 || p > 65535 {
			return c, fmt.Errorf("無效端口：%d", p)
		}
	}
	return c, nil
}
