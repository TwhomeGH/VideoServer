package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/TwhomeGH/VideoServer/internal/atomicfile"
)

// RuntimeState 保存啟動成功時的監聽結果，不取代管理者設定或 NodeID。
type RuntimeState struct {
	SchemaVersion      int       `json:"schemaVersion"`
	Service            string    `json:"service"`
	Host               string    `json:"host"`
	LastSuccessfulPort int       `json:"lastSuccessfulPort"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// NetworkRuntime 是本次啟動的狀態檔診斷，區分實際端口與是否成功保存。
type NetworkRuntime struct {
	StateFile       string `json:"stateFile"`
	RememberEnabled bool   `json:"rememberEnabled"`
	PreviousPort    int    `json:"previousPort"`
	ActivePort      int    `json:"activePort"`
	Saved           bool   `json:"saved"`
	Warning         string `json:"warning,omitempty"`
}

// LoadRuntimeState 讀取指定 service 的狀態；首次啟動不存在時回傳空狀態。
// 損壞、版本不支援或服務身分不符會回傳錯誤，呼叫端應保留原檔。
func LoadRuntimeState(path, service string) (RuntimeState, error) {
	var state RuntimeState
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return state, err
	}
	if info.Size() > 65536 {
		return state, fmt.Errorf("執行狀態檔超過 64 KiB")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return RuntimeState{}, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return RuntimeState{}, fmt.Errorf("執行狀態必須只有一個 JSON 物件")
	}
	if state.SchemaVersion != 1 || state.Service != service || state.LastSuccessfulPort < 1 || state.LastSuccessfulPort > 65535 || state.UpdatedAt.IsZero() {
		return RuntimeState{}, fmt.Errorf("執行狀態版本、服務或端口無效")
	}
	return state, nil
}

// SaveRuntimeState 原子保存 state；呼叫端須先完成服務初始化並建立資料目錄。
func SaveRuntimeState(path string, state RuntimeState) error {
	if state.SchemaVersion != 1 || state.Service == "" || state.LastSuccessfulPort < 1 || state.LastSuccessfulPort > 65535 || state.UpdatedAt.IsZero() {
		return fmt.Errorf("執行狀態無效")
	}
	return atomicfile.WriteJSON(path, state)
}

func candidatePorts(config Config, state RuntimeState) []int {
	allowed := append([]int{config.PreferredPort}, config.FallbackPorts...)
	result := make([]int, 0, len(allowed))
	if config.RememberLastSuccessfulPort && config.Host == state.Host {
		for _, port := range allowed {
			if port == state.LastSuccessfulPort && port != 0 {
				result = append(result, port)
				break
			}
		}
	}
	seen := map[int]bool{}
	for _, port := range result {
		seen[port] = true
	}
	for _, port := range allowed {
		if !seen[port] {
			result = append(result, port)
			seen[port] = true
		}
	}
	return result
}
