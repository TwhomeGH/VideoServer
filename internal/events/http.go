package events

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const heartbeatInterval = 15 * time.Second
const maxStreamDuration = 30 * time.Minute

// Register 掛上受管理授權保護的 SSE 端點。
func Register(mux *http.ServeMux, hub *Hub) {
	mux.HandleFunc("GET /api/v1/events", hub.serve)
}

func (h *Hub) serve(w http.ResponseWriter, r *http.Request) {
	flusher := http.NewResponseController(w)
	after := lastEventID(r)
	subscriber, replay, replayOK := h.SubscribeAfter(after)
	defer h.Unsubscribe(subscriber)

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 3000\n\n")
	fmt.Fprintf(w, "event: ready\ndata: {\"latest\":%d}\n\n", h.Latest())
	if !replayOK {
		fmt.Fprint(w, "event: reset\ndata: {\"reason\":\"replay-unavailable\"}\n\n")
	} else {
		for _, event := range replay {
			writeEvent(w, event)
		}
	}
	_ = flusher.Flush()

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	deadline := time.NewTimer(maxStreamDuration)
	defer deadline.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-deadline.C:
			return
		case <-subscriber.Done():
			fmt.Fprint(w, "event: reset\ndata: {\"reason\":\"subscriber-overflow\"}\n\n")
			_ = flusher.Flush()
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": keepalive\n\n")
			_ = flusher.Flush()
		case event := <-subscriber.Events():
			writeEvent(w, event)
			_ = flusher.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, event Event) {
	fmt.Fprintf(w, "id: %d\nevent: %s\ndata: {}\n\n", event.ID, event.Type)
}

func lastEventID(r *http.Request) uint64 {
	value := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if value == "" {
		value = strings.TrimSpace(r.URL.Query().Get("lastEventId"))
	}
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0
	}
	return id
}
