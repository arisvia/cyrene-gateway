package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// RequestEvent represents a real-time request event for SSE streaming.
type RequestEvent struct {
	RequestID string `json:"requestId,omitempty"`
	Sequence  int64  `json:"sequence,omitempty"`
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

const requestIDHeader = "X-Cyrene-Request-ID"

// ensureRequestID creates one gateway-owned ID and reuses it through routing,
// streaming, fallback and usage events. SanitizeInternalHeaders strips any
// client supplied X-Cyrene-Request-ID first, so the value cached here is
// always gateway-generated and correlates every event of one request.
func ensureRequestID(r *http.Request) string {
	if r == nil {
		return uuid.NewString()
	}
	if id := r.Header.Get(requestIDHeader); id != "" {
		return id
	}
	id := uuid.NewString()
	r.Header.Set(requestIDHeader, id)
	return id
}

func (s *Server) publishRequestEvent(r *http.Request, ev RequestEvent) {
	if s == nil || s.Events == nil {
		return
	}
	if ev.RequestID == "" {
		ev.RequestID = ensureRequestID(r)
	}
	if ev.Timestamp == "" {
		ev.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	s.Events.Publish(ev)
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

	primeSSE(w, flusher)

	ch, done := s.Events.Subscribe()
	defer s.Events.Unsubscribe(ch)

	ctx := r.Context()
	// Send initial heartbeat. A single ticker avoids allocating a new timer on every loop.
	writeSSE(w, flusher, "connected", `{"ok":true}`)
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case ev := <-ch:
			data, _ := json.Marshal(ev)
			evtName := ev.Type
			if evtName == "" {
				evtName = "request"
			}
			writeSSE(w, flusher, evtName, string(data))
		case <-heartbeat.C:
			// Heartbeat to keep tunnels and reverse proxies from idling out.
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

// SSE priming: Cloudflare's edge and Nginx hold text/event-stream responses in
// their write buffers until a threshold (typically 4-8KB) is reached. Padding
// the handshake with SSE comment lines flushes the buffer immediately so the
// stream is live from the first byte. Comment lines are ignored by EventSource.
const (
	ssePrimeBytes = 8192
	ssePrimeChunk = 2048
)

// primeSSE commits SSE response headers and flushes an 8KB comment pad in 2KB
// chunks, keeping each line short enough for proxies with per-line limits.
func primeSSE(w http.ResponseWriter, flusher http.Flusher) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	pad := strings.Repeat(" ", ssePrimeChunk-4)
	for remaining := ssePrimeBytes; remaining > 0; remaining -= ssePrimeChunk {
		w.Write([]byte(": " + pad + "\n\n"))
		flusher.Flush()
	}
}
