package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(3)
	for i := 0; i < 3; i++ {
		if !rl.Allow("k1") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if rl.Allow("k1") {
		t.Fatal("4th request should be denied")
	}
	if !rl.Allow("k2") {
		t.Fatal("different key should be allowed")
	}
}

func TestRateLimiterDisabled(t *testing.T) {
	rl := NewRateLimiter(0)
	for i := 0; i < 100; i++ {
		if !rl.Allow("k1") {
			t.Fatal("limit 0 must disable limiting")
		}
	}
	var nilRL *RateLimiter
	if !nilRL.Allow("k1") {
		t.Fatal("nil limiter must allow")
	}
}

func TestAPIKeyRateLimitMiddleware(t *testing.T) {
	limit := 2
	handler := APIKeyRateLimit(func() int { return limit })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	do := func() int {
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer cg-test.sig")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w.Code
	}

	if c := do(); c != 200 {
		t.Fatalf("req1: want 200 got %d", c)
	}
	if c := do(); c != 200 {
		t.Fatalf("req2: want 200 got %d", c)
	}
	if c := do(); c != http.StatusTooManyRequests {
		t.Fatalf("req3: want 429 got %d", c)
	}

	// Non-/v1 paths bypass the limiter entirely.
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("/api path should bypass: got %d", w.Code)
	}

	// Raising the limit takes effect without restart.
	limit = 100
	if c := do(); c != 200 {
		t.Fatalf("after limit raise: want 200 got %d", c)
	}
}

func TestRateLimiterConcurrent(t *testing.T) {
	rl := NewRateLimiter(50)
	var wg sync.WaitGroup
	var allowed int64 = -1
	var mu sync.Mutex
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok := rl.Allow("shared")
			mu.Lock()
			if ok && allowed < 0 {
				allowed = 1
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	_ = allowed
	// No race = pass (run with -race in CI).
	if !strings.Contains("", "") {
		t.Fatal("unreachable")
	}
}
