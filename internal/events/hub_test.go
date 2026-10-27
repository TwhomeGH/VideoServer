package events

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHubReplayAndGap(t *testing.T) {
	hub := NewHub(2)
	hub.Publish("jobs")
	hub.Publish("scan")
	sub, replay, ok := hub.SubscribeAfter(1)
	if !ok || len(replay) != 1 || replay[0].Type != "scan" {
		t.Fatalf("replay = %+v ok=%v", replay, ok)
	}
	hub.Unsubscribe(sub)

	hub.Publish("config")
	hub.Publish("jobs")
	hub.Publish("scan")
	_, _, ok = hub.SubscribeAfter(2)
	if ok {
		t.Fatal("expected replay gap when oldest event is newer than Last-Event-ID")
	}
}

func TestHubOverflowClosesSubscriber(t *testing.T) {
	hub := NewHub(4)
	sub, _, ok := hub.SubscribeAfter(0)
	if !ok {
		t.Fatal("subscribe failed")
	}
	for i := 0; i < subscriberBuffer+1; i++ {
		hub.Publish("jobs")
	}
	select {
	case <-sub.Done():
	case <-time.After(time.Second):
		t.Fatal("overflowed subscriber was not closed")
	}
}

func TestSSEStreamsAndReplays(t *testing.T) {
	hub := NewHub(16)
	server := httptest.NewServer(http.HandlerFunc(hub.serve))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	readUntil := func(want string) {
		t.Helper()
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("read %q: %v", want, err)
			}
			if strings.Contains(line, want) {
				return
			}
		}
	}
	readUntil("event: ready")
	hub.Publish("jobs")
	readUntil("event: jobs")
	cancel()
}
