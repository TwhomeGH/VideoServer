package events

import "sync"

const subscriberBuffer = 64

// Event 是 SSE 的最小事件：ID 供 Last-Event-ID 重播，Type 由前端訂閱。
type Event struct {
	ID   uint64
	Type string
}

// Subscriber 是單一 SSE 連線；events 滿載或 hub 關閉時 done 會關閉。
type Subscriber struct {
	events chan Event
	done   chan struct{}
	once   sync.Once
}

func (s *Subscriber) Events() <-chan Event  { return s.events }
func (s *Subscriber) Done() <-chan struct{} { return s.done }
func (s *Subscriber) close() {
	s.once.Do(func() { close(s.done) })
}

// Hub 保存有界事件環與訂閱者；發布不阻塞掃描或工作執行。
type Hub struct {
	mu          sync.Mutex
	nextID      uint64
	capacity    int
	ring        []Event
	subscribers map[*Subscriber]struct{}
}

func NewHub(capacity int) *Hub {
	if capacity < 1 {
		capacity = 256
	}
	return &Hub{capacity: capacity, subscribers: map[*Subscriber]struct{}{}}
}

// Publish 新增事件；慢速訂閱者超過緩衝時會被關閉，前端重連後取得 reset。
func (h *Hub) Publish(eventType string) {
	h.mu.Lock()
	h.nextID++
	event := Event{ID: h.nextID, Type: eventType}
	h.ring = append(h.ring, event)
	if len(h.ring) > h.capacity {
		h.ring = append([]Event(nil), h.ring[len(h.ring)-h.capacity:]...)
	}
	for subscriber := range h.subscribers {
		select {
		case subscriber.events <- event:
		default:
			delete(h.subscribers, subscriber)
			subscriber.close()
		}
	}
	h.mu.Unlock()
}

// SubscribeAfter 原子地加入訂閱並取得 after 之後的重播；gap=false 代表需要快照。
func (h *Hub) SubscribeAfter(after uint64) (*Subscriber, []Event, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	replay := []Event{}
	ok := true
	if after > h.nextID {
		ok = false
	} else if after > 0 {
		if len(h.ring) > 0 && after+1 < h.ring[0].ID {
			ok = false
		}
		if ok {
			for _, event := range h.ring {
				if event.ID > after {
					replay = append(replay, event)
				}
			}
		}
	}
	subscriber := &Subscriber{events: make(chan Event, subscriberBuffer), done: make(chan struct{})}
	h.subscribers[subscriber] = struct{}{}
	return subscriber, replay, ok
}

func (h *Hub) Unsubscribe(subscriber *Subscriber) {
	h.mu.Lock()
	delete(h.subscribers, subscriber)
	h.mu.Unlock()
	subscriber.close()
}

func (h *Hub) Latest() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.nextID
}
