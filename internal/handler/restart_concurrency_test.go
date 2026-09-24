package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRestartRejectsDuplicateRequests(t *testing.T) {
	release := make(chan struct{})
	done := make(chan struct{}, 2)
	var calls atomic.Int32
	s := &Server{RestartFunc: func() error {
		calls.Add(1)
		<-release
		done <- struct{}{}
		return errors.New("spawn failed; listener restored")
	}}
	first := httptest.NewRecorder()
	s.handleSystemRestart(first, httptest.NewRequest(http.MethodPost, "/api/system/restart", nil))
	second := httptest.NewRecorder()
	s.handleSystemRestart(second, httptest.NewRequest(http.MethodPost, "/api/system/restart", nil))
	close(release)
	if first.Code != http.StatusOK || second.Code != http.StatusConflict {
		t.Errorf("expected first=200 duplicate=409; got %d/%d", first.Code, second.Code)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("restart callback was not called")
	}
	if calls.Load() != 1 {
		t.Fatalf("spawn called %d times", calls.Load())
	}
}
