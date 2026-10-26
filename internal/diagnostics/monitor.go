// Package diagnostics 提供目前 DVN 服務的程序與 HTTP 工作負載快照。
package diagnostics

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Snapshot 的 CPU 以單核心 100% 計算，記憶體為 Go runtime；流量僅含此服務 HTTP 本文。
type Snapshot struct {
	Service               string    `json:"service"`
	StartedAt             time.Time `json:"startedAt"`
	SampledAt             time.Time `json:"sampledAt"`
	UptimeSeconds         float64   `json:"uptimeSeconds"`
	CPUPercent            *float64  `json:"cpuPercent"`
	CPUError              string    `json:"cpuError,omitempty"`
	LogicalCPUs           int       `json:"logicalCPUs"`
	GoMaxProcs            int       `json:"goMaxProcs"`
	HeapAllocBytes        uint64    `json:"heapAllocBytes"`
	GoReservedBytes       uint64    `json:"goReservedBytes"`
	Goroutines            int       `json:"goroutines"`
	Requests              uint64    `json:"requests"`
	InFlight              int64     `json:"inFlight"`
	Errors5xx             uint64    `json:"errors5xx"`
	ReceivedBytes         uint64    `json:"receivedBytes"`
	SentBytes             uint64    `json:"sentBytes"`
	ReceiveBytesPerSecond float64   `json:"receiveBytesPerSecond"`
	SendBytesPerSecond    float64   `json:"sendBytesPerSecond"`
	RequestsPerSecond     float64   `json:"requestsPerSecond"`
	Tasks                 any       `json:"tasks"`
	TasksError            string    `json:"tasksError,omitempty"`
}

// Monitor 保存程序生命週期內的計數；重新啟動後歸零，不保存請求 URL 或內容。
type Monitor struct {
	service                              string
	started                              time.Time
	tasks                                func(context.Context) (any, error)
	requests, received, sent, errors5xx  atomic.Uint64
	inFlight                             atomic.Int64
	mu                                   sync.Mutex
	sample                               Snapshot
	lastTime                             time.Time
	lastCPU                              time.Duration
	cpuReady                             bool
	lastReceived, lastSent, lastRequests uint64
}

// New 建立 service 的監看器；tasks 可為 nil，否則在已授權的查詢中取得工作快照。
func New(service string, tasks func(context.Context) (any, error)) *Monitor {
	now := time.Now()
	cpu, err := processCPU()
	return &Monitor{service: service, started: now, tasks: tasks, lastTime: now, lastCPU: cpu, cpuReady: err == nil}
}

// Run 每兩秒取樣，直到 ctx 取消；每個 Monitor 僅能啟動一次。
func (m *Monitor) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.collect(now)
		}
	}
}

func (m *Monitor) collect(now time.Time) {
	cpu, err := processCPU()
	received, sent, requests := m.received.Load(), m.sent.Load(), m.requests.Load()
	m.mu.Lock()
	defer m.mu.Unlock()
	seconds := now.Sub(m.lastTime).Seconds()
	if seconds <= 0 {
		return
	}
	m.sample.CPUPercent = nil
	m.sample.CPUError = ""
	if err != nil {
		m.sample.CPUError = err.Error()
	} else if m.cpuReady {
		percent := max(0, (cpu-m.lastCPU).Seconds()/seconds*100)
		m.sample.CPUPercent = &percent
	}
	m.sample.SampledAt = now.UTC()
	m.sample.ReceiveBytesPerSecond = float64(received-m.lastReceived) / seconds
	m.sample.SendBytesPerSecond = float64(sent-m.lastSent) / seconds
	m.sample.RequestsPerSecond = float64(requests-m.lastRequests) / seconds
	m.lastTime, m.lastCPU, m.cpuReady = now, cpu, err == nil
	m.lastReceived, m.lastSent, m.lastRequests = received, sent, requests
}

// Snapshot 取得即時程序數值及最近一次速率樣本；工作讀取錯誤另以 TasksError 回報。
func (m *Monitor) Snapshot(ctx context.Context) Snapshot {
	m.mu.Lock()
	result := m.sample
	m.mu.Unlock()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	result.Service, result.StartedAt, result.UptimeSeconds = m.service, m.started.UTC(), time.Since(m.started).Seconds()
	result.LogicalCPUs, result.GoMaxProcs, result.Goroutines = runtime.NumCPU(), runtime.GOMAXPROCS(0), runtime.NumGoroutine()
	result.HeapAllocBytes, result.GoReservedBytes = memory.HeapAlloc, memory.Sys
	result.Requests, result.InFlight, result.Errors5xx = m.requests.Load(), m.inFlight.Load(), m.errors5xx.Load()
	result.ReceivedBytes, result.SentBytes = m.received.Load(), m.sent.Load()
	if m.tasks != nil {
		var err error
		result.Tasks, err = m.tasks(ctx)
		if err != nil {
			result.TasksError = "無法讀取背景工作，請稍後重試"
		}
	}
	return result
}

// Register 加入監看 API；呼叫端必須套用服務既有的登入授權 middleware。
func (m *Monitor) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/monitor", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(m.Snapshot(r.Context()))
	})
}
