package auth

import (
	"encoding/json"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Register 將登入、登出、session 與改密碼路由加入 mux。
func (m *Manager) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/auth/session", m.handleSession)
	mux.HandleFunc("POST /api/v1/auth/login", m.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", m.handleLogout)
	mux.HandleFunc("POST /api/v1/auth/password", m.handlePassword)
}

// Middleware 保護管理 API；靜態頁、健康檢查、登入與 session 查詢保持公開。
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if !strings.HasPrefix(path, "/api/") || path == "/api/v1/status" ||
			path == "/api/v1/auth/session" || path == "/api/v1/auth/login" {
			next.ServeHTTP(w, r)
			return
		}
		session, ok := m.session(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "需要登入"})
			return
		}
		m.refreshCookie(w, r, session)
		next.ServeHTTP(w, r)
	})
}

func (m *Manager) handleSession(w http.ResponseWriter, r *http.Request) {
	session, ok := m.session(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	session = m.refreshCookie(w, r, session)
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true, "expiresAt": session.ExpiresAt, "absoluteExpiresAt": session.AbsoluteExpiresAt,
	})
}

func (m *Manager) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "不允許跨站登入"})
		return
	}
	var request struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	key := remoteKey(r)
	if allowed, wait := m.limiter.allowed(key, m.now()); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(math.Ceil(wait.Seconds())))))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "登入嘗試過多，請稍後再試"})
		return
	}
	if !m.Verify(request.Password) {
		m.limiter.fail(key, m.now())
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "密碼錯誤"})
		return
	}
	m.limiter.reset(key)
	if err := m.setSessionCookie(w, r); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "無法建立登入狀態"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

func (m *Manager) handleLogout(w http.ResponseWriter, r *http.Request) {
	if err := m.RotateSecret(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "無法清除登入狀態"})
		return
	}
	clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

func (m *Manager) handlePassword(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "不允許跨站請求"})
		return
	}
	var request struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if err := m.SetPassword(request.CurrentPassword, request.NewPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := m.setSessionCookie(w, r); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "密碼已更新，但無法建立登入狀態"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

// sameOrigin 拒絕瀏覽器跨站請求；沒有 Origin 的命令列用戶端不受影響。
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host == r.Host
}

// decodeJSON 限制請求大小與格式，與其他管理 API 的行為一致。
func decodeJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if contentType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "請使用 application/json"})
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "請求 JSON 格式無效"})
		return false
	}
	if decoder.Decode(new(any)) != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "請求必須只包含一個 JSON 物件"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
