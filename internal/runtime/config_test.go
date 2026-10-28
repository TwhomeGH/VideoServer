package runtime

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func baseTestConfig() Config {
	return Config{
		Host: "0.0.0.0", PreferredPort: 18610, FallbackPorts: []int{18611, 18612},
		WebDirectory: "/app/web", DataDirectory: "/data", AutoScan: true,
		ScanIntervalSeconds: 30, ScanIdleMaxSeconds: 21600, FileStableSeconds: 10, AutoBackfill: false,
		BackfillIntervalSeconds: 60, BackfillDailyLimit: 50, BackfillDailyGiB: 20, HashLimitMiBps: 64, RememberLastSuccessfulPort: true,
		FFprobePath: "ffprobe", ProbeTimeoutSeconds: 60,
	}
}

func configBody(value Config) string {
	body, _ := json.Marshal(map[string]any{
		"host": value.Host, "preferredPort": value.PreferredPort, "fallbackPorts": value.FallbackPorts,
		"rememberLastSuccessfulPort": value.RememberLastSuccessfulPort, "autoScan": value.AutoScan,
		"scanIntervalSeconds": value.ScanIntervalSeconds, "scanIdleMaxSeconds": value.ScanIdleMaxSeconds, "fileStableSeconds": value.FileStableSeconds,
		"autoBackfill": value.AutoBackfill, "backfillIntervalSeconds": value.BackfillIntervalSeconds,
		"backfillDailyLimit": value.BackfillDailyLimit, "backfillDailyGiB": value.BackfillDailyGiB,
		"hashLimitMiBps": value.HashLimitMiBps,
		"autoCover":      value.AutoCover, "coverIntervalSeconds": value.CoverIntervalSeconds,
	})
	return string(body)
}

func TestOverrideRoundTrip(t *testing.T) {
	base := baseTestConfig()
	next := base
	next.Host = "127.0.0.1"
	next.ScanIntervalSeconds = 60
	next.FallbackPorts = []int{19001}
	override := BuildOverride(base, next)
	if override.Host == nil || *override.Host != "127.0.0.1" || override.ScanIntervalSeconds == nil ||
		*override.ScanIntervalSeconds != 60 || override.FallbackPorts == nil {
		t.Fatalf("unexpected override: %+v", override)
	}
	if override.AutoScan != nil || override.PreferredPort != nil {
		t.Fatalf("unchanged fields were included: %+v", override)
	}
	path := filepath.Join(t.TempDir(), "config.override.json")
	if err := SaveOverride(path, override); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOverride(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Apply(base), next) {
		t.Fatalf("merged config = %+v, want %+v", loaded.Apply(base), next)
	}
	next.Host = base.Host
	next.ScanIntervalSeconds = base.ScanIntervalSeconds
	next.FallbackPorts = base.FallbackPorts
	if err := SaveOverride(path, BuildOverride(base, next)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("empty override file was not removed")
	}
}

func TestConfigStateUpdateAndReset(t *testing.T) {
	directory := t.TempDir()
	base := baseTestConfig()
	base.DataDirectory = directory
	overridePath := filepath.Join(directory, "config.override.json")
	state := NewConfigState(base, base, Override{}, "/config.json", overridePath, nil, nil)
	mux := http.NewServeMux()
	state.Register(mux)

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/config", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET config status = %d", response.Code)
	}
	var view configView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.RestartRequired || view.Source != "/config.json" || !reflect.DeepEqual(view.Effective, base) {
		t.Fatalf("unexpected initial view: %+v", view)
	}

	next := base
	next.ScanIntervalSeconds = 90
	next.AutoScan = false
	response = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBufferString(configBody(next)))
	request.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("PUT config status = %d body=%s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.RestartRequired || view.Effective.ScanIntervalSeconds != 90 || view.Effective.AutoScan {
		t.Fatalf("update not reflected: %+v", view)
	}
	if _, err := os.Stat(overridePath); err != nil {
		t.Fatalf("override file missing: %v", err)
	}

	invalid := next
	invalid.PreferredPort = 70000
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/api/v1/config", bytes.NewBufferString(configBody(invalid)))
	request.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid config status = %d", response.Code)
	}

	view = configView{}
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/config/overrides", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("DELETE overrides status = %d", response.Code)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(view.Effective, base) || !view.Overrides.Empty() {
		t.Fatalf("reset did not restore base: %+v", view)
	}
	if _, err := os.Stat(overridePath); !os.IsNotExist(err) {
		t.Fatal("override file was not removed")
	}
}

func TestConfigStateRestartConfirmation(t *testing.T) {
	base := baseTestConfig()
	restarted := make(chan struct{}, 1)
	state := NewConfigState(base, base, Override{}, "/config.json", filepath.Join(t.TempDir(), "override.json"), nil,
		func() { restarted <- struct{}{} })
	mux := http.NewServeMux()
	state.Register(mux)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/restart", bytes.NewBufferString(`{"confirm":false}`))
	request.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unconfirmed restart status = %d", response.Code)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/system/restart", bytes.NewBufferString(`{"confirm":true}`))
	request.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("restart status = %d", response.Code)
	}
	select {
	case <-restarted:
	case <-time.After(2 * time.Second):
		t.Fatal("restart callback was not called")
	}
}
