package protocol

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// Register 加入版本能力公告與 OpenAPI 路由；OpenAPI 仍由呼叫端的管理授權保護。
func Register(mux *http.ServeMux, service string) {
	mux.HandleFunc("GET /api/v1/protocol", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(map[string]any{
			"service": service, "serviceType": ServiceType(service), "contentTypes": ContentTypes(service), "protocolVersion": Current,
			"minProtocolVersion": MinSupported, "maxProtocolVersion": MaxSupported,
			"supportedVersions": SupportedVersions(), "capabilities": Capabilities(service),
			"capabilityVersions": CapabilityVersions(service), "openapi": "/api/v1/openapi.json",
		})
	})
	mux.HandleFunc("GET /api/v1/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		value, err := Document(service)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err != nil {
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(map[string]string{"error": "無法讀取 API 規格"})
			return
		}
		w.Write(value)
	})
}

// Middleware 在 /api/ 回覆協定版本；可選請求標頭若無效則回應 400，不相容則回應 409。
// 不帶標頭時維持現有用戶端行為；只有檢查成功才會執行 next。
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set(Header, strconv.Itoa(Current))
		values, present := r.Header[http.CanonicalHeaderKey(Header)]
		if !present {
			next.ServeHTTP(w, r)
			return
		}
		version := 0
		var err error
		if len(values) == 1 {
			version, err = strconv.Atoi(values[0])
		}
		status, code, message := 400, "invalid_protocol_version", "協定版本標頭必須是正整數"
		if len(values) == 1 && err == nil && version > 0 && strconv.Itoa(version) == values[0] {
			if Compatible(version) {
				next.ServeHTTP(w, r)
				return
			}
			status, code, message = 409, "protocol_incompatible", "用戶端與服務端協定版本不相容，請更新或使用支援的版本"
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]any{"error": message, "code": code, "protocolVersion": Current, "minProtocolVersion": MinSupported, "maxProtocolVersion": MaxSupported, "supportedVersions": SupportedVersions()})
	})
}
