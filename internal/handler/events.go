package handler

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// RequestEvent represents a real-time request event for SSE streaming.
type RequestEvent struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Status    string `json:"status,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	Prompt    int    `json:"promptTokens,omitempty"`
	Compl     int    `json:"completionTokens,omitempty"`
	LatencyMs int64  `json:"latencyMs,omitempty"`
}

// EventBroadcaster is a simple pub/sub for request events.
type EventBroadcaster struct {
	subs map[chan RequestEvent]chan struct{}
	mu   sync.RWMutex
}

func NewEventBroadcaster() *EventBroadcaster {
	return &EventBroadcaster{
		subs: make(map[chan RequestEvent]chan struct{}),
	}
}

// Subscribe returns an events channel and a done channel.
func (b *EventBroadcaster) Subscribe() (chan RequestEvent, chan struct{}) {
	ch := make(chan RequestEvent, 64)
	done := make(chan struct{})
	b.mu.Lock()
	b.subs[ch] = done
	b.mu.Unlock()
	return ch, done
}

// Unsubscribe removes a subscriber channel and closes its done signal.
func (b *EventBroadcaster) Unsubscribe(ch chan RequestEvent) {
	b.mu.Lock()
	done, ok := b.subs[ch]
	if ok {
		delete(b.subs, ch)
		close(done)
	}
	b.mu.Unlock()
}

// Publish sends an event to all subscribers (non-blocking, drops if buffer full).
func (b *EventBroadcaster) Publish(ev RequestEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// handleUsageStream is an SSE endpoint that streams real-time request events.
func (s *Server) handleUsageStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, done := s.Events.Subscribe()
	defer s.Events.Unsubscribe(ch)

	ctx := r.Context()
	// Send initial heartbeat
	writeSSE(w, flusher, "connected", `{"ok":true}`)

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case ev := <-ch:
			data, _ := json.Marshal(ev)
			writeSSE(w, flusher, "request", string(data))
		case <-time.After(30 * time.Second):
			// Heartbeat to keep connection alive
			writeSSE(w, flusher, "ping", `{}`)
		}
	}
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event, data string) {
	if event != "" {
		w.Write([]byte("event: " + event + "\n"))
	}
	w.Write([]byte("data: " + data + "\n\n"))
	flusher.Flush()
}
