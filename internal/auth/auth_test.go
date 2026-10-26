package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	previous := defaultIterations
	defaultIterations = 1000
	code := m.Run()
	defaultIterations = previous
	os.Exit(code)
}

func testManager(t *testing.T) *Manager {
	t.Helper()
	manager, initial, err := Open(t.TempDir(), "test-password-123")
	if err != nil {
		t.Fatal(err)
	}
	if initial != "" {
		t.Fatalf("unexpected generated password: %q", initial)
	}
	return manager
}

func login(t *testing.T, handler http.Handler, password, remote string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"password": password})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = remote
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response.Result()
}

func protectedRequest(handler http.Handler, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestOpenGeneratesPasswordOnce(t *testing.T) {
	directory := t.TempDir()
	manager, initial, err := Open(directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(initial) < passwordMinimum || !manager.Verify(initial) {
		t.Fatalf("generated password not usable")
	}
	reopened, second, err := Open(directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if second != "" || !reopened.Verify(initial) {
		t.Fatalf("generated password was not persisted")
	}
}

func TestEnvironmentPasswordUpdatesStoredCredential(t *testing.T) {
	directory := t.TempDir()
	first, _, err := Open(directory, "first-password-123")
	if err != nil {
		t.Fatal(err)
	}
	cookie, _, err := first.newSession()
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := Open(directory, "second-password-456")
	if err != nil {
		t.Fatal(err)
	}
	if second.Verify("first-password-123") || !second.Verify("second-password-456") {
		t.Fatalf("environment password did not replace old credential")
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	request.AddCookie(&http.Cookie{Name: cookieName, Value: cookie})
	if second.Authenticated(request) {
		t.Fatalf("old session remained valid after password rotation")
	}
}

func TestSessionAndMiddleware(t *testing.T) {
	manager := testManager(t)
	mux := http.NewServeMux()
	manager.Register(mux)
	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /api/v1/protected", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := manager.Middleware(mux)

	if response := protectedRequest(handler, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request status = %d", response.Code)
	}
	response := login(t, handler, "test-password-123", "192.0.2.10:1000")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", response.StatusCode)
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatalf("session cookie missing or not HttpOnly: %+v", cookies)
	}
	if response := protectedRequest(handler, cookies[0]); response.Code != http.StatusOK {
		t.Fatalf("authenticated request status = %d", response.Code)
	}
	tampered := *cookies[0]
	tampered.Value += "x"
	if response := protectedRequest(handler, &tampered); response.Code != http.StatusUnauthorized {
		t.Fatalf("tampered session accepted: %d", response.Code)
	}
	if response := protectedRequest(handler, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("missing session accepted")
	}
}

func TestSessionExpiresByIdleAndAbsolute(t *testing.T) {
	manager := testManager(t)
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	token, session, err := manager.newSession()
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	request.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	if !manager.Authenticated(request) {
		t.Fatal("fresh session rejected")
	}
	now = session.ExpiresAt.Add(time.Second)
	if manager.Authenticated(request) {
		t.Fatal("idle-expired session accepted")
	}

	now = time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	absoluteToken, _, err := manager.encodeSession(now.Add(48*time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	absoluteRequest := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	absoluteRequest.AddCookie(&http.Cookie{Name: cookieName, Value: absoluteToken})
	if manager.Authenticated(absoluteRequest) {
		t.Fatal("absolute-expired session accepted")
	}
}

func TestSessionRefreshExtendsIdleButNotAbsolute(t *testing.T) {
	manager := testManager(t)
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	token, session, err := manager.newSession()
	if err != nil {
		t.Fatal(err)
	}
	now = session.ExpiresAt.Add(-sessionRefreshWindow / 2)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	request.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	recorder := httptest.NewRecorder()
	refreshed := manager.refreshCookie(recorder, request, session)
	if !refreshed.ExpiresAt.After(session.ExpiresAt) {
		t.Fatalf("idle expiry was not extended: %v -> %v", session.ExpiresAt, refreshed.ExpiresAt)
	}
	if !refreshed.AbsoluteExpiresAt.Equal(session.AbsoluteExpiresAt) {
		t.Fatal("absolute expiry changed during refresh")
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value == token {
		t.Fatal("refreshed cookie was not written")
	}
	next := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	next.AddCookie(cookies[0])
	if !manager.Authenticated(next) {
		t.Fatal("refreshed cookie is not usable")
	}

	now = session.AbsoluteExpiresAt.Add(-time.Minute)
	refreshed = manager.refreshCookie(httptest.NewRecorder(), request, session)
	if refreshed.ExpiresAt.After(session.AbsoluteExpiresAt) {
		t.Fatal("refresh exceeded absolute expiry")
	}
}

func TestLoginRateLimit(t *testing.T) {
	manager := testManager(t)
	mux := http.NewServeMux()
	manager.Register(mux)
	handler := manager.Middleware(mux)
	for index := 0; index < 5; index++ {
		if response := login(t, handler, "wrong-password", "192.0.2.20:1000"); response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d", index, response.StatusCode)
		}
	}
	if response := login(t, handler, "test-password-123", "192.0.2.20:1000"); response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("rate limit status = %d", response.StatusCode)
	}
	if response := login(t, handler, "test-password-123", "192.0.2.21:1000"); response.StatusCode != http.StatusOK {
		t.Fatalf("different address blocked: %d", response.StatusCode)
	}
}

func TestPasswordChangeInvalidatesOldPassword(t *testing.T) {
	manager := testManager(t)
	mux := http.NewServeMux()
	manager.Register(mux)
	handler := manager.Middleware(mux)
	response := login(t, handler, "test-password-123", "192.0.2.30:1000")
	cookie := response.Cookies()[0]
	body := strings.NewReader(`{"currentPassword":"test-password-123","newPassword":"new-password-456"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password", body)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	changed := httptest.NewRecorder()
	handler.ServeHTTP(changed, request)
	if changed.Code != http.StatusOK {
		t.Fatalf("password change status = %d body=%s", changed.Code, changed.Body.String())
	}
	if manager.Verify("test-password-123") || !manager.Verify("new-password-456") {
		t.Fatalf("password was not updated")
	}
	if manager.Authenticated(request) {
		t.Fatalf("old session remained valid after password change")
	}
	newCookie := changed.Result().Cookies()[0]
	refreshed := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	refreshed.AddCookie(newCookie)
	if !manager.Authenticated(refreshed) {
		t.Fatalf("new session cookie was not usable")
	}
}
