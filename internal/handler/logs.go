package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// LogRecord represents a structured gateway runtime log line.
type LogRecord struct {
	Timestamp string         `json:"time"`
	Level     string         `json:"level"`
	Message   string         `json:"msg"`
	Attrs     map[string]any `json:"attrs,omitempty"`
}

// LogRingBuffer stores the latest N runtime logs in memory and broadcasts new logs via SSE.
type LogRingBuffer struct {
	mu      sync.RWMutex
	records []LogRecord
	maxSize int
	subs    map[chan LogRecord]struct{}
}

// GlobalLogBuffer is the singleton log ring buffer accessible to slog handler and API.
var GlobalLogBuffer = NewLogRingBuffer(1000)

func NewLogRingBuffer(maxSize int) *LogRingBuffer {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &LogRingBuffer{
		records: make([]LogRecord, 0, maxSize),
		maxSize: maxSize,
		subs:    make(map[chan LogRecord]struct{}),
	}
}

func (b *LogRingBuffer) Add(rec LogRecord) {
	b.mu.Lock()
	if len(b.records) >= b.maxSize {
		b.records = b.records[1:]
	}
	b.records = append(b.records, rec)
	b.mu.Unlock()

	// Broadcast to active SSE subscribers
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- rec:
		default:
		}
	}
}

func (b *LogRingBuffer) GetAll() []LogRecord {
	b.mu.RLock()
	defer b.mu.RUnlock()
	res := make([]LogRecord, len(b.records))
	copy(res, b.records)
	return res
}

func (b *LogRingBuffer) Subscribe() chan LogRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan LogRecord, 128)
	b.subs[ch] = struct{}{}
	return ch
}

func (b *LogRingBuffer) Unsubscribe(ch chan LogRecord) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subs, ch)
	close(ch)
}

// BroadcastLogHandler is a slog.Handler that writes to a base handler and the GlobalLogBuffer.
type BroadcastLogHandler struct {
	base slog.Handler
}

func NewBroadcastLogHandler(base slog.Handler) *BroadcastLogHandler {
	return &BroadcastLogHandler{base: base}
}

func (h *BroadcastLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *BroadcastLogHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := make(map[string]any)
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})

	GlobalLogBuffer.Add(LogRecord{
		Timestamp: r.Time.Format(time.RFC3339Nano),
		Level:     r.Level.String(),
		Message:   r.Message,
		Attrs:     attrs,
	})

	return h.base.Handle(ctx, r)
}

func (h *BroadcastLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &BroadcastLogHandler{base: h.base.WithAttrs(attrs)}
}

func (h *BroadcastLogHandler) WithGroup(name string) slog.Handler {
	return &BroadcastLogHandler{base: h.base.WithGroup(name)}
}

// handleSystemLogs returns the recent in-memory server logs.
func (s *Server) handleSystemLogs(w http.ResponseWriter, r *http.Request) {
	logs := GlobalLogBuffer.GetAll()
	writeJSON(w, http.StatusOK, map[string]any{
		"logs":  logs,
		"count": len(logs),
	})
}

// handleSystemLogsStream streams live server logs via SSE.
func (s *Server) handleSystemLogsStream(w http.ResponseWriter, r *http.Request) {
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

	ch := GlobalLogBuffer.Subscribe()
	defer GlobalLogBuffer.Unsubscribe(ch)

	ctx := r.Context()
	writeSSE(w, flusher, "connected", `{"ok":true}`)

	for {
		select {
		case <-ctx.Done():
			return
		case rec, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(rec)
			writeSSE(w, flusher, "log", string(data))
		case <-time.After(30 * time.Second):
			writeSSE(w, flusher, "ping", `{}`)
		}
	}
}
