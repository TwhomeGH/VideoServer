package diagnostics

import (
	"io"
	"net/http"
	"sync/atomic"
)

type countedBody struct {
	io.ReadCloser
	count *atomic.Uint64
}

func (b *countedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.count.Add(uint64(n))
	return n, err
}

type countedResponse struct {
	http.ResponseWriter
	count  *atomic.Uint64
	status int
}

func (w *countedResponse) WriteHeader(status int) {
	if status >= 100 && status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *countedResponse) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.count.Add(uint64(n))
	return n, err
}

// Unwrap 讓 http.ResponseController 仍可存取底層連線與串流能力。
func (w *countedResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Middleware 記錄所有 HTTP 請求與實際讀寫的本文位元組，包含授權拒絕、播放和監看本身。
// 不代表網卡總流量，亦不包含 TLS、HTTP 標頭或其他程序的網路活動。
func (m *Monitor) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.inFlight.Add(1)
		response := &countedResponse{ResponseWriter: w, count: &m.sent}
		defer func() {
			m.inFlight.Add(-1)
			m.requests.Add(1)
			if response.status >= 500 {
				m.errors5xx.Add(1)
			}
		}()
		if r.Body != nil {
			r.Body = &countedBody{ReadCloser: r.Body, count: &m.received}
		}
		next.ServeHTTP(response, r)
	})
}
