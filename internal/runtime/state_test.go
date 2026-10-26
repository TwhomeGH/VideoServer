package runtime

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestRememberedPortOrderRespectsConfiguration(t *testing.T) {
	config := Config{Host: "127.0.0.1", PreferredPort: 18610, FallbackPorts: []int{18611, 18612, 18611}, RememberLastSuccessfulPort: true}
	state := RuntimeState{Host: config.Host, LastSuccessfulPort: 18611}
	if got := candidatePorts(config, state); !reflect.DeepEqual(got, []int{18611, 18610, 18612}) {
		t.Fatalf("unexpected priority: %v", got)
	}
	state.LastSuccessfulPort = 19000
	if got := candidatePorts(config, state); !reflect.DeepEqual(got, []int{18610, 18611, 18612}) {
		t.Fatalf("unconfigured port accepted: %v", got)
	}
	state.LastSuccessfulPort = 18611
	state.Host = "0.0.0.0"
	if candidatePorts(config, state)[0] != 18610 {
		t.Fatal("state from another interface was reused")
	}
	state.Host = config.Host
	config.RememberLastSuccessfulPort = false
	if candidatePorts(config, state)[0] != 18610 {
		t.Fatal("disabled memory was used")
	}
}

func TestStateRoundTripAndOccupiedRememberedPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.json")
	state, err := LoadRuntimeState(path, "video-node")
	if err != nil || state.LastSuccessfulPort != 0 {
		t.Fatal("first boot must allow missing state")
	}
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port
	saved := RuntimeState{SchemaVersion: 1, Service: "video-node", Host: "127.0.0.1", LastSuccessfulPort: port, UpdatedAt: time.Now().UTC()}
	if err := SaveRuntimeState(path, saved); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRuntimeState(path, "video-node")
	if err != nil || loaded != saved {
		t.Fatalf("state round trip: %+v %v", loaded, err)
	}
	config := Config{Host: "127.0.0.1", PreferredPort: 0, FallbackPorts: []int{port}, RememberLastSuccessfulPort: true}
	listener, err := listenPorts(config, candidatePorts(config, loaded))
	if err != nil {
		t.Fatal(err)
	}
	listener.Close()
	// 原本端口釋放後，即使 preferred 可用也要優先沿用記憶端口。
	occupied.Close()
	listener, err = listenPorts(config, candidatePorts(config, loaded))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if listener.Addr().(*net.TCPAddr).Port != port {
		t.Fatal("restart did not reuse the last successful port")
	}
	if _, err := LoadRuntimeState(path, "central"); err == nil {
		t.Fatal("another service identity accepted")
	}
}

func TestDamagedStateAndInvalidConfigPreserveFiles(t *testing.T) {
	directory := t.TempDir()
	statePath := filepath.Join(directory, "runtime.json")
	damaged := []byte(`{"schemaVersion":1,`)
	if err := os.WriteFile(statePath, damaged, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRuntimeState(statePath, "video-node"); err == nil {
		t.Fatal("damaged state accepted")
	}
	after, _ := os.ReadFile(statePath)
	if !bytes.Equal(after, damaged) {
		t.Fatal("loading damaged state overwrote diagnostics")
	}
	config, err := LoadConfig("", 18610)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.json")
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(configPath)
	config.PreferredPort = -1
	if err := SaveConfig(configPath, config); err == nil {
		t.Fatal("invalid config saved")
	}
	after, _ = os.ReadFile(configPath)
	if !bytes.Equal(original, after) {
		t.Fatal("validation failure overwrote config")
	}
	config.PreferredPort = 18612
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatal(err)
	}
	roundTrip, err := LoadConfig(configPath, 18610)
	if err != nil || roundTrip.PreferredPort != 18612 {
		t.Fatal("config update did not persist")
	}
}

func TestFixedPortConfigDisablesMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"fallbackPorts":[],"rememberLastSuccessfulPort":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(path, 18610)
	if err != nil || config.RememberLastSuccessfulPort {
		t.Fatal("fixed container port must not use memory")
	}
}

func TestServiceDefaultDataDirectories(t *testing.T) {
	central, err := LoadConfig("", 18620, "central")
	if err != nil {
		t.Fatal(err)
	}
	worker, err := LoadConfig("", 18630, "transcode-worker")
	if err != nil {
		t.Fatal(err)
	}
	if central.DataDirectory != filepath.Join("data", "central") || worker.DataDirectory != filepath.Join("data", "transcode-worker") {
		t.Fatal("services share the default state directory")
	}
	if _, err := LoadConfig("", 18610, "../outside"); err == nil {
		t.Fatal("invalid service accepted")
	}
}
