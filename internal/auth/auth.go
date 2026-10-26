// Package auth 提供管理介面的單一管理員密碼、簽章 session 與 API 保護。
package auth

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TwhomeGH/VideoServer/internal/atomicfile"
)

const (
	fileVersion     = 1
	saltLength      = 16
	hashLength      = 32
	secretLength    = 32
	passwordMinimum = 12
	cookieName      = "dvn_session"

	// sessionIdleLifetime 是無操作後失效時間；有效請求會刷新。
	sessionIdleLifetime = 7 * 24 * time.Hour
	// sessionAbsoluteLifetime 是即使持續使用也必須重新登入的上限。
	sessionAbsoluteLifetime = 90 * 24 * time.Hour
	// sessionRefreshWindow 在剩餘時間低於一半時刷新 cookie，避免每個請求都寫入。
	sessionRefreshWindow = sessionIdleLifetime / 2
)

// Session 是通過簽章驗證後的有效期限資訊。
type Session struct {
	ExpiresAt         time.Time
	AbsoluteExpiresAt time.Time
}

// defaultIterations 是 PBKDF2-HMAC-SHA256 的預設迭代次數；測試可暫時調低。
var defaultIterations = 600_000

// stored 是 auth.json 的磁碟格式；敏感值以 base64 保存。
type stored struct {
	Version       int    `json:"version"`
	Salt          string `json:"salt"`
	Hash          string `json:"hash"`
	Iterations    int    `json:"iterations"`
	SessionSecret string `json:"sessionSecret"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// Manager 保存目前管理員憑證與 session 簽章金鑰。
type Manager struct {
	mu         sync.RWMutex
	path       string
	salt       []byte
	hash       []byte
	iterations int
	secret     []byte
	createdAt  time.Time
	updatedAt  time.Time
	limiter    *limiter
	now        func() time.Time
}

// limiter 針對登入失敗來源進行短期鎖定，避免無限猜測密碼。
type limiter struct {
	mu       sync.Mutex
	attempts map[string]attempt
}

type attempt struct {
	failures    int
	lockedUntil time.Time
}

// Open 載入或建立 dataDir/auth.json。
// 沒有環境密碼時會產生一次性隨機密碼並回傳；呼叫端應只顯示一次。
// 有環境密碼時會以它為準，若與檔案不同則更新並使既有 session 失效。
func Open(dataDir, envPassword string) (manager *Manager, initialPassword string, err error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, "", fmt.Errorf("建立資料目錄失敗：%w", err)
	}
	path := filepath.Join(dataDir, "auth.json")
	if data, readErr := os.ReadFile(path); readErr == nil {
		manager, err = decodeState(path, data)
		if err != nil {
			return nil, "", err
		}
		if envPassword != "" {
			if err := validatePassword(envPassword); err != nil {
				return nil, "", fmt.Errorf("DVN_ADMIN_PASSWORD 無效：%w", err)
			}
			if !manager.Verify(envPassword) {
				if err := manager.replacePassword(envPassword); err != nil {
					return nil, "", err
				}
			}
		}
		return manager, "", nil
	} else if !os.IsNotExist(readErr) {
		return nil, "", fmt.Errorf("讀取管理憑證失敗：%w", readErr)
	}

	password := envPassword
	generated := ""
	if password == "" {
		password, err = randomPassword()
		if err != nil {
			return nil, "", err
		}
		generated = password
	}
	if err := validatePassword(password); err != nil {
		return nil, "", fmt.Errorf("管理密碼無效：%w", err)
	}
	manager = &Manager{path: path, limiter: newLimiter(), now: time.Now}
	if err := manager.replacePassword(password); err != nil {
		return nil, "", err
	}
	return manager, generated, nil
}

// decodeState 解析 auth.json 並驗證必要欄位。
func decodeState(path string, data []byte) (*Manager, error) {
	var value stored
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("管理憑證格式無效：%w", err)
	}
	if value.Version != fileVersion || value.Iterations < 1 {
		return nil, fmt.Errorf("管理憑證版本不支援")
	}
	salt, err := base64.RawStdEncoding.DecodeString(value.Salt)
	if err != nil || len(salt) < saltLength {
		return nil, fmt.Errorf("管理憑證 salt 無效")
	}
	hash, err := base64.RawStdEncoding.DecodeString(value.Hash)
	if err != nil || len(hash) < hashLength {
		return nil, fmt.Errorf("管理憑證 hash 無效")
	}
	secret, err := base64.RawStdEncoding.DecodeString(value.SessionSecret)
	if err != nil || len(secret) < secretLength {
		return nil, fmt.Errorf("管理憑證 session secret 無效")
	}
	createdAt, err := time.Parse(time.RFC3339, value.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("管理憑證建立時間無效：%w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339, value.UpdatedAt)
	if err != nil {
		updatedAt = createdAt
	}
	return &Manager{
		path: path, salt: salt, hash: hash, iterations: value.Iterations, secret: secret,
		createdAt: createdAt, updatedAt: updatedAt, limiter: newLimiter(), now: time.Now,
	}, nil
}

// replacePassword 建立新的 salt、密碼雜湊與 session secret，並原子保存。
func (m *Manager) replacePassword(password string) error {
	salt := make([]byte, saltLength)
	secret := make([]byte, secretLength)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("產生管理憑證失敗：%w", err)
	}
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("產生 session 金鑰失敗：%w", err)
	}
	hash, err := pbkdf2.Key(sha256.New, password, salt, defaultIterations, hashLength)
	if err != nil {
		return fmt.Errorf("計算密碼雜湊失敗：%w", err)
	}
	now := m.now().UTC()
	m.mu.Lock()
	m.salt, m.hash, m.iterations, m.secret = salt, hash, defaultIterations, secret
	if m.createdAt.IsZero() {
		m.createdAt = now
	}
	m.updatedAt = now
	m.mu.Unlock()
	return m.save()
}

// Verify 以常數時間比對密碼；讀取憑證時不阻塞其他請求。
func (m *Manager) Verify(password string) bool {
	m.mu.RLock()
	salt, hash, iterations := append([]byte(nil), m.salt...), append([]byte(nil), m.hash...), m.iterations
	m.mu.RUnlock()
	candidate, err := pbkdf2.Key(sha256.New, password, salt, iterations, hashLength)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(candidate, hash) == 1
}

// save 將目前憑證原子寫入 auth.json。
func (m *Manager) save() error {
	m.mu.RLock()
	value := stored{
		Version: fileVersion,
		Salt: base64.RawStdEncoding.EncodeToString(m.salt), Hash: base64.RawStdEncoding.EncodeToString(m.hash),
		Iterations: m.iterations, SessionSecret: base64.RawStdEncoding.EncodeToString(m.secret),
		CreatedAt: m.createdAt.Format(time.RFC3339), UpdatedAt: m.updatedAt.Format(time.RFC3339),
	}
	m.mu.RUnlock()
	return atomicfile.WriteJSON(m.path, value)
}

// newSession 建立新的閒置期限與簽章 cookie 值。
func (m *Manager) newSession() (string, Session, error) {
	now := m.now()
	return m.encodeSession(now.Add(sessionIdleLifetime), now.Add(sessionAbsoluteLifetime))
}

// encodeSession 建立簽章；absolute 不會因活動刷新而延長。
func (m *Manager) encodeSession(expiry, absolute time.Time) (string, Session, error) {
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return "", Session{}, err
	}
	payload := strconv.FormatInt(expiry.Unix(), 10) + "." + strconv.FormatInt(absolute.Unix(), 10) + "." +
		base64.RawURLEncoding.EncodeToString(nonce)
	m.mu.RLock()
	signature := sign(m.secret, []byte(payload))
	m.mu.RUnlock()
	token := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(signature)
	return token, Session{ExpiresAt: expiry, AbsoluteExpiresAt: absolute}, nil
}

// session 驗證簽章、閒置期限與絕對期限。
func (m *Manager) session(r *http.Request) (Session, bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		return Session{}, false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return Session{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Session{}, false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Session{}, false
	}
	m.mu.RLock()
	valid := hmac.Equal(signature, sign(m.secret, payload))
	m.mu.RUnlock()
	if !valid {
		return Session{}, false
	}
	values := strings.SplitN(string(payload), ".", 3)
	if len(values) != 3 {
		return Session{}, false
	}
	expiry, err := strconv.ParseInt(values[0], 10, 64)
	if err != nil {
		return Session{}, false
	}
	absolute, err := strconv.ParseInt(values[1], 10, 64)
	if err != nil {
		return Session{}, false
	}
	now := m.now()
	if !now.Before(time.Unix(expiry, 0)) || !now.Before(time.Unix(absolute, 0)) {
		return Session{}, false
	}
	return Session{ExpiresAt: time.Unix(expiry, 0), AbsoluteExpiresAt: time.Unix(absolute, 0)}, true
}

// Authenticated 驗證目前請求是否帶有仍有效的 session。
func (m *Manager) Authenticated(r *http.Request) bool {
	_, ok := m.session(r)
	return ok
}

// refreshCookie 在閒置期限剩餘不到一半時延長，但絕不超過絕對期限。
func (m *Manager) refreshCookie(w http.ResponseWriter, r *http.Request, current Session) Session {
	now := m.now()
	if now.Add(sessionRefreshWindow).Before(current.ExpiresAt) || !now.Before(current.AbsoluteExpiresAt) {
		return current
	}
	expiry := now.Add(sessionIdleLifetime)
	if expiry.After(current.AbsoluteExpiresAt) {
		expiry = current.AbsoluteExpiresAt
	}
	token, session, err := m.encodeSession(expiry, current.AbsoluteExpiresAt)
	if err != nil {
		return current
	}
	m.writeSessionCookie(w, r, token, session)
	return session
}

// sign 回傳 HMAC-SHA256 簽章。
func sign(secret, payload []byte) []byte {
	hash := hmac.New(sha256.New, secret)
	hash.Write(payload)
	return hash.Sum(nil)
}

// RotateSecret 使目前所有 session 失效，並保存新金鑰。
func (m *Manager) RotateSecret() error {
	secret := make([]byte, secretLength)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	m.mu.Lock()
	previous, previousUpdated := m.secret, m.updatedAt
	m.secret, m.updatedAt = secret, m.now().UTC()
	m.mu.Unlock()
	if err := m.save(); err != nil {
		m.mu.Lock()
		m.secret, m.updatedAt = previous, previousUpdated
		m.mu.Unlock()
		return err
	}
	return nil
}

// SetPassword 驗證目前密碼後更新管理密碼，並讓所有既有 session 失效。
func (m *Manager) SetPassword(current, next string) error {
	if !m.Verify(current) {
		return errors.New("目前密碼不正確")
	}
	if err := validatePassword(next); err != nil {
		return err
	}
	return m.replacePassword(next)
}

func randomPassword() (string, error) {
	value := make([]byte, 18)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func validatePassword(value string) error {
	if len(value) < passwordMinimum {
		return fmt.Errorf("密碼長度至少 %d 個字元", passwordMinimum)
	}
	if len(value) > 200 {
		return fmt.Errorf("密碼長度不可超過 200 個字元")
	}
	return nil
}

// setSessionCookie 建立新的 session cookie。
func (m *Manager) setSessionCookie(w http.ResponseWriter, r *http.Request) error {
	token, session, err := m.newSession()
	if err != nil {
		return err
	}
	m.writeSessionCookie(w, r, token, session)
	return nil
}

// writeSessionCookie 套用 HttpOnly、SameSite=Strict 與目前到期時間。
func (m *Manager) writeSessionCookie(w http.ResponseWriter, r *http.Request, token string, session Session) {
	maxAge := int(session.ExpiresAt.Sub(m.now()).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: r.TLS != nil, Expires: session.ExpiresAt, MaxAge: maxAge,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: r.TLS != nil, MaxAge: -1, Expires: time.Unix(0, 0),
	})
}

// newLimiter 建立登入失敗計數器。
func newLimiter() *limiter { return &limiter{attempts: map[string]attempt{}} }

// allowed 回傳來源是否可嘗試登入，以及需要等待的時間。
func (l *limiter) allowed(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	value, ok := l.attempts[key]
	if !ok || value.lockedUntil.IsZero() || !now.Before(value.lockedUntil) {
		return true, 0
	}
	return false, value.lockedUntil.Sub(now)
}

// fail 記錄一次失敗；達門檻後鎖定一段時間。
func (l *limiter) fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	value := l.attempts[key]
	value.failures++
	if value.failures >= 5 {
		value.failures = 0
		value.lockedUntil = now.Add(30 * time.Second)
	}
	l.attempts[key] = value
	if len(l.attempts) > 1024 {
		for current, item := range l.attempts {
			if item.lockedUntil.IsZero() || now.After(item.lockedUntil.Add(time.Minute)) {
				delete(l.attempts, current)
			}
		}
	}
}

func (l *limiter) reset(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

func remoteKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
