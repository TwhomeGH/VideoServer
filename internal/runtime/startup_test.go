package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRuntimeHelperProcess(t *testing.T) {
	path := os.Getenv("DVN_RUNTIME_TEST_CONFIG")
	if path == "" {
		return
	}
	flag.CommandLine = flag.NewFlagSet("runtime-helper", flag.ExitOnError)
	os.Args = []string{os.Args[0], "-config", path}
	os.Exit(Main("central", 18620))
}

func startTestService(t *testing.T, configPath string) (int, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRuntimeHelperProcess$")
	env := make([]string, 0, len(os.Environ())+2)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "DVN_ADMIN_PASSWORD=") {
			env = append(env, value)
		}
	}
	command.Env = append(env, "DVN_RUNTIME_TEST_CONFIG="+configPath, "DVN_ADMIN_PASSWORD=test-password-123")
	output, err := command.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	var once sync.Once
	stop := func() { once.Do(func() { command.Process.Kill(); command.Wait(); cancel() }) }
	t.Cleanup(stop)
	type startupResult struct {
		port int
		err  error
	}
	ready := make(chan startupResult, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			var line struct {
				Msg           string `json:"msg"`
				ListenAddress string `json:"listenAddress"`
			}
			if json.Unmarshal(scanner.Bytes(), &line) == nil && line.Msg == "服務啟動" {
				_, port, _ := net.SplitHostPort(line.ListenAddress)
				value, _ := strconv.Atoi(port)
				ready <- startupResult{port: value}
				return
			}
		}
		if err := scanner.Err(); err != nil {
			ready <- startupResult{err: err}
			return
		}
		ready <- startupResult{}
	}()
	select {
	case result := <-ready:
		if result.err != nil {
			stop()
			t.Fatalf("read service startup output: %v", result.err)
		}
		if result.port == 0 {
			stop()
			t.Fatal("service exited before becoming ready")
		}
		return result.port, stop
	case <-ctx.Done():
		stop()
		t.Fatal("service startup timed out")
	}
	return 0, stop
}

func TestStartupPersistsFallbackAndKeepsConfigUnchanged(t *testing.T) {
	occupied, err := listenTCP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	preferred := occupied.Addr().(*net.TCPAddr).Port
	spare, err := listenTCP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fallback := spare.Addr().(*net.TCPAddr).Port
	spare.Close()
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.json")
	config, err := LoadConfig("", 18620)
	if err != nil {
		t.Fatal(err)
	}
	config.Host = "127.0.0.1"
	config.PreferredPort = preferred
	config.FallbackPorts = []int{fallback}
	config.DataDirectory = filepath.Join(directory, "data")
	config.AutoScan = false
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(configPath)
	port, stop := startTestService(t, configPath)
	if port != fallback {
		t.Fatalf("expected fallback %d, got %d", fallback, port)
	}
	stop()
	occupied.Close()
	port, stop = startTestService(t, configPath)
	if port != fallback {
		t.Fatal("restart returned to preferred instead of remembered port")
	}
	client := &http.Client{Timeout: 3 * time.Second}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Jar = jar
	unauthorized, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/api/v1/monitor")
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatal("monitor did not require login")
	}
	login, err := client.Post("http://127.0.0.1:"+strconv.Itoa(port)+"/api/v1/auth/login",
		"application/json", strings.NewReader(`{"password":"test-password-123"}`))
	if err != nil {
		t.Fatal(err)
	}
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", login.StatusCode)
	}
	monitor, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/api/v1/monitor")
	if err != nil {
		t.Fatal(err)
	}
	monitor.Body.Close()
	if monitor.StatusCode != http.StatusOK {
		t.Fatal("authorized monitor unavailable")
	}
	response, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/api/v1/config")
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Network NetworkRuntime `json:"network"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if !body.Network.Saved || body.Network.PreviousPort != fallback {
		t.Fatalf("unexpected diagnostics: %+v", body.Network)
	}
	stop()
	after, _ := os.ReadFile(configPath)
	if string(original) != string(after) {
		t.Fatal("runtime bookkeeping changed administrator config")
	}
	// 損壞狀態只告警；服務仍以目前設定啟動，並保留原始損壞內容。
	statePath := filepath.Join(config.DataDirectory, "runtime.json")
	damaged := []byte(`{"broken":`)
	if err := os.WriteFile(statePath, damaged, 0600); err != nil {
		t.Fatal(err)
	}
	port, stop = startTestService(t, configPath)
	if port != preferred {
		t.Fatal("damaged state did not fall back to configured preference")
	}
	stop()
	after, _ = os.ReadFile(statePath)
	if string(after) != string(damaged) {
		t.Fatal("damaged diagnostic state was overwritten")
	}
}
