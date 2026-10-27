package protocol

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed openapi.json
var document []byte

// Document 回傳只保留 service 可用路由的 OpenAPI JSON；來源解析失敗時回傳錯誤。
// 共用 schema 保留，能力公告不代表跨服務註冊或身份驗證已實作。
func Document(service string) ([]byte, error) {
	if len(Capabilities(service)) == 0 {
		return nil, fmt.Errorf("未知服務：%s", service)
	}
	var spec map[string]any
	if err := json.Unmarshal(document, &spec); err != nil {
		return nil, err
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("OpenAPI 缺少 paths")
	}
	for path, raw := range paths {
		item := raw.(map[string]any)
		if only, _ := item["x-dvn-service"].(string); only != "" && only != service {
			delete(paths, path)
		}
	}
	// 隱藏目前服務沒有操作的分組，避免 Central／Worker 文件出現空的影片功能。
	usedTags := map[string]bool{}
	for _, raw := range paths {
		for _, value := range raw.(map[string]any) {
			operation, ok := value.(map[string]any)
			if !ok {
				continue
			}
			if tags, ok := operation["tags"].([]any); ok {
				for _, tag := range tags {
					usedTags[tag.(string)] = true
				}
			}
		}
	}
	if tags, ok := spec["tags"].([]any); ok {
		filtered := []any{}
		for _, tag := range tags {
			if usedTags[tag.(map[string]any)["name"].(string)] {
				filtered = append(filtered, tag)
			}
		}
		spec["tags"] = filtered
	}
	spec["x-dvn-service"] = service
	return json.Marshal(spec)
}
