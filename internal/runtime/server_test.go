package runtime

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TwhomeGH/VideoServer/pkg/buildinfo"
)

func TestListenFallback(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port
	// 第二個候選也被占用時，必須回報失敗而非假裝啟動成功。
	if listener, err := Listen(Config{Host: "127.0.0.1", PreferredPort: port, FallbackPorts: []int{port}}); err == nil {
		listener.Close()
		t.Fatal("expected exhausted ports error")
	}
	listener, err := Listen(Config{Host: "127.0.0.1", PreferredPort: port, FallbackPorts: []int{0}})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if listener.Addr().(*net.TCPAddr).Port == port {
		t.Fatal("occupied port selected")
	}
}

func TestStatusAndUnknownAPI(t *testing.T) {
	handler := Handler(buildinfo.Current("video-node"), "127.0.0.1:8081", t.TempDir())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/status", nil))
	var body struct {
		Build         buildinfo.Identity `json:"build"`
		ListenAddress string             `json:"listenAddress"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || body.Build.Service != "video-node" || body.ListenAddress != "127.0.0.1:8081" {
		t.Fatalf("unexpected status: %s", response.Body)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/unknown", nil))
	if response.Code != 404 {
		t.Fatalf("unexpected status %d", response.Code)
	}
}

func TestConfigRejectsInvalidPort(t *testing.T) {
	if _, err := LoadConfig("", 65535); err == nil {
		t.Fatal("invalid fallback ports accepted")
	}
}

func TestEffectiveConfigSource(t *testing.T) {
	config := Config{Host: "0.0.0.0", PreferredPort: 18610, DataDirectory: "/data/test", WebDirectory: "/app/web", FallbackPorts: []int{}}
	handler := Handler(buildinfo.Current("video-node"), "0.0.0.0:18610", t.TempDir(), func(mux *http.ServeMux) { registerConfig(mux, config, "/config/video-node.json") })
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/config", nil))
	var body struct {
		Source    string `json:"source"`
		Effective Config `json:"effective"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || body.Source != "/config/video-node.json" || body.Effective.DataDirectory != config.DataDirectory {
		t.Fatalf("unexpected config: %s", response.Body)
	}
}
