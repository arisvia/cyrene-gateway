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
	Prompt    int    `json:"promptTokens,omitempty"`
	Compl     int    `json:"completionTokens,omitempty"`
	LatencyMs int64  `json:"latencyMs,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
}

// EventBroadcaster is a simple pub/sub for request events.
type EventBroadcaster struct {
	mu   sync.RWMutex
	subs map[chan RequestEvent]struct{}
}

func NewEventBroadcaster() *EventBroadcaster {
	return &EventBroadcaster{
		subs: make(map[chan RequestEvent]struct{}),
	}
}

// Subscribe returns a channel that receives events. Caller must call Unsubscribe when done.
func (b *EventBroadcaster) Subscribe() chan RequestEvent {
	ch := make(chan RequestEvent, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (b *EventBroadcaster) Unsubscribe(ch chan RequestEvent) {
	b.mu.Lock()
	delete(b.subs, ch)
	b.mu.Unlock()
	close(ch)
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

	ch := s.Events.Subscribe()
	defer s.Events.Unsubscribe(ch)

	ctx := r.Context()
	// Send initial heartbeat
	writeSSE(w, flusher, "connected", `{"ok":true}`)

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
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
