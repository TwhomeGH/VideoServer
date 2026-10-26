package diagnostics

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMiddlewareCountsPayloadAndErrors(t *testing.T) {
	m := New("video-node", nil)
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.Snapshot(r.Context()).InFlight != 1 {
			t.Error("missing active request")
		}
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
		}
		w.WriteHeader(503)
		w.WriteHeader(200)
		io.WriteString(w, "offline")
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("POST", "/test", strings.NewReader("input")))
	m.collect(m.lastTime.Add(2 * time.Second))
	value := m.Snapshot(context.Background())
	if response.Code != 503 || value.ReceivedBytes != 5 || value.SentBytes != 7 || value.Errors5xx != 1 || value.Requests != 1 || value.InFlight != 0 {
		t.Fatalf("bad counters: %+v", value)
	}
	if value.ReceiveBytesPerSecond != 2.5 || value.SendBytesPerSecond != 3.5 || value.RequestsPerSecond != 0.5 {
		t.Fatalf("bad rates: %+v", value)
	}
	if value.CPUPercent == nil || *value.CPUPercent < 0 || value.LogicalCPUs < 1 || value.HeapAllocBytes == 0 {
		t.Fatalf("missing process metrics: %+v", value)
	}
	m.collect(m.lastTime.Add(2 * time.Second))
	value = m.Snapshot(context.Background())
	if value.SendBytesPerSecond != 0 || value.ReceiveBytesPerSecond != 0 {
		t.Fatal("idle rate not reset")
	}
}

func TestMonitorEndpointAndTaskFailure(t *testing.T) {
	m := New("central", func(context.Context) (any, error) { return nil, errors.New("private database path") })
	mux := http.NewServeMux()
	m.Register(mux)
	response := httptest.NewRecorder()
	m.Middleware(mux).ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/monitor", nil))
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Body.String(), "tasksError") || strings.Contains(response.Body.String(), "private database path") {
		t.Fatalf("bad diagnostics response: %s", response.Body.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitor failed to stop")
	}
}
