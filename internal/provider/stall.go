package provider

import (
	"errors"
	"io"
	"sync"
	"time"
)

// ErrStreamStalled is returned when an upstream stream emits no data for the stall timeout.
var ErrStreamStalled = errors.New("upstream stream stalled: no data received within timeout")

// DefaultStreamStallTimeout is the maximum idle duration allowed between consecutive
// read events before an active streaming connection is aborted as unresponsive.
const DefaultStreamStallTimeout = 5 * time.Minute

// stallTimeoutReader wraps an io.ReadCloser with an idle timer that resets on every
// successful read. If no data arrives within timeout, the reader aborts and closes the underlying stream.
type stallTimeoutReader struct {
	rc      io.ReadCloser
	timeout time.Duration
	timer   *time.Timer
	mu      sync.Mutex
	stalled bool
	closed  bool
}

// NewStallTimeoutReader creates a reader that aborts if the upstream stream becomes idle for timeout.
func NewStallTimeoutReader(rc io.ReadCloser, timeout time.Duration) io.ReadCloser {
	if rc == nil || timeout <= 0 {
		return rc
	}
	s := &stallTimeoutReader{
		rc:      rc,
		timeout: timeout,
	}
	s.timer = time.AfterFunc(timeout, s.onStall)
	return s
}

func (s *stallTimeoutReader) onStall() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.stalled = true
	s.mu.Unlock()
	_ = s.rc.Close()
}

func (s *stallTimeoutReader) Read(p []byte) (int, error) {
	s.mu.Lock()
	if s.stalled {
		s.mu.Unlock()
		return 0, ErrStreamStalled
	}
	s.mu.Unlock()

	n, err := s.rc.Read(p)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stalled {
		return n, ErrStreamStalled
	}
	if !s.closed && s.timer != nil {
		s.timer.Reset(s.timeout)
	}
	return n, err
}

func (s *stallTimeoutReader) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.mu.Unlock()
	return s.rc.Close()
}
