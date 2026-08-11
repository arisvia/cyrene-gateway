package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIsQoderPAT(t *testing.T) {
	tests := []struct {
		token string
		want  bool
	}{
		{"pt-abc123", true},
		{"jt-abc123", false},
		{"dt-abc123", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsQoderPAT(tt.token); got != tt.want {
			t.Errorf("IsQoderPAT(%q) = %v, want %v", tt.token, got, tt.want)
		}
	}
}

// Job-token traffic must hit api2 (api3 rejects jt- with 403); device tokens
// stay on api3 (9router@d433c0b2).
func TestQoderURLRoutingByToken(t *testing.T) {
	if got := QoderChatURLForToken("jt-test"); !strings.Contains(got, "api2.qoder.sh") {
		t.Errorf("jt- chat URL should use api2, got %q", got)
	}
	if got := QoderChatURLForToken("dt-test"); !strings.Contains(got, "api3.qoder.sh") {
		t.Errorf("dt- chat URL should use api3, got %q", got)
	}
	if got := QoderModelListURL("jt-test"); !strings.Contains(got, "api2.qoder.sh") {
		t.Errorf("jt- model list URL should use api2, got %q", got)
	}
	if got := QoderModelListURL("dt-test"); !strings.Contains(got, "api3.qoder.sh") {
		t.Errorf("dt- model list URL should use api3, got %q", got)
	}
}

// qoderPATMockServers stands up exchange + userinfo mocks and points the
// package-level URLs at them.
func qoderPATMockServers(t *testing.T, exchangeStatus int, jobToken string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/jobToken/exchange", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("exchange: expected POST, got %s", r.Method)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if !strings.HasPrefix(body["personal_token"], "pt-") {
			t.Errorf("exchange: personal_token missing pt- prefix: %q", body["personal_token"])
		}
		w.WriteHeader(exchangeStatus)
		if exchangeStatus == http.StatusOK {
			json.NewEncoder(w).Encode(map[string]any{
				"token":      jobToken,
				"expires_at": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
			})
		}
	})
	mux.HandleFunc("/api/v1/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+jobToken {
			t.Errorf("userinfo: expected Bearer %s, got %q", jobToken, got)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "user-from-pat"})
	})
	srv := httptest.NewServer(mux)

	origExchange, origUserInfo := QoderJobTokenExchangeURL, QoderPATUserInfoURL
	QoderJobTokenExchangeURL = srv.URL + "/api/v1/jobToken/exchange"
	QoderPATUserInfoURL = srv.URL + "/api/v1/userinfo"
	t.Cleanup(func() {
		QoderJobTokenExchangeURL = origExchange
		QoderPATUserInfoURL = origUserInfo
		srv.Close()
	})
	return srv
}

func TestResolveQoderCredential_PATExchange(t *testing.T) {
	pat := "pt-test-exchange-1"
	InvalidateQoderPATCache(pat)
	qoderPATMockServers(t, http.StatusOK, "jt-exchanged")

	cred, err := ResolveQoderCredential(pat, "", nil)
	if err != nil {
		t.Fatalf("ResolveQoderCredential failed: %v", err)
	}
	if cred.AccessToken != "jt-exchanged" {
		t.Errorf("expected job token jt-exchanged, got %q", cred.AccessToken)
	}
	if cred.UserID != "user-from-pat" {
		t.Errorf("expected userId from userinfo, got %q", cred.UserID)
	}
	if cred.ExpiresAt.Before(time.Now()) {
		t.Error("expiry should be in the future")
	}
	InvalidateQoderPATCache(pat)
}

// Cached result is returned without a second exchange.
func TestResolveQoderCredential_Cached(t *testing.T) {
	pat := "pt-test-cached-1"
	InvalidateQoderPATCache(pat)
	exchanges := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/jobToken/exchange", func(w http.ResponseWriter, r *http.Request) {
		exchanges++
		json.NewEncoder(w).Encode(map[string]any{
			"token":      "jt-cached",
			"expires_at": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		})
	})
	mux.HandleFunc("/api/v1/userinfo", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": "user-cached"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	origExchange, origUserInfo := QoderJobTokenExchangeURL, QoderPATUserInfoURL
	QoderJobTokenExchangeURL = srv.URL + "/api/v1/jobToken/exchange"
	QoderPATUserInfoURL = srv.URL + "/api/v1/userinfo"
	defer func() {
		QoderJobTokenExchangeURL = origExchange
		QoderPATUserInfoURL = origUserInfo
	}()

	for i := 0; i < 3; i++ {
		cred, err := ResolveQoderCredential(pat, "", srv.Client())
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
		if cred.AccessToken != "jt-cached" {
			t.Errorf("call %d: unexpected token %q", i, cred.AccessToken)
		}
	}
	if exchanges != 1 {
		t.Errorf("expected exactly 1 exchange, got %d", exchanges)
	}
	InvalidateQoderPATCache(pat)
}

func TestResolveQoderCredential_ExchangeFailure(t *testing.T) {
	pat := "pt-test-fail-1"
	InvalidateQoderPATCache(pat)
	qoderPATMockServers(t, http.StatusForbidden, "")

	_, err := ResolveQoderCredential(pat, "", nil)
	if err == nil {
		t.Fatal("expected error for failed exchange")
	}
	if !strings.Contains(err.Error(), "PAT exchange failed") {
		t.Errorf("unexpected error message: %v", err)
	}
	InvalidateQoderPATCache(pat)
}

// Non-PAT tokens pass through unchanged (device/job tokens are used directly).
func TestResolveQoderCredential_Passthrough(t *testing.T) {
	cred, err := ResolveQoderCredential("dt-device-token", "user-42", nil)
	if err != nil {
		t.Fatalf("passthrough failed: %v", err)
	}
	if cred.AccessToken != "dt-device-token" || cred.UserID != "user-42" {
		t.Errorf("unexpected passthrough result: %+v", cred)
	}
}

// Concurrent callers for the same PAT must not each trigger an exchange.
func TestResolveQoderCredential_ConcurrentDedup(t *testing.T) {
	pat := "pt-test-concurrent-1"
	InvalidateQoderPATCache(pat)
	exchanges := 0
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/jobToken/exchange", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		exchanges++
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]any{
			"token":      "jt-concurrent",
			"expires_at": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		})
	})
	mux.HandleFunc("/api/v1/userinfo", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": "user-concurrent"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	origExchange, origUserInfo := QoderJobTokenExchangeURL, QoderPATUserInfoURL
	QoderJobTokenExchangeURL = srv.URL + "/api/v1/jobToken/exchange"
	QoderPATUserInfoURL = srv.URL + "/api/v1/userinfo"
	defer func() {
		QoderJobTokenExchangeURL = origExchange
		QoderPATUserInfoURL = origUserInfo
	}()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cred, err := ResolveQoderCredential(pat, "", srv.Client())
			if err != nil {
				t.Errorf("concurrent resolve failed: %v", err)
				return
			}
			if cred.AccessToken != "jt-concurrent" {
				t.Errorf("unexpected token %q", cred.AccessToken)
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if exchanges != 1 {
		t.Errorf("expected 1 exchange for concurrent callers, got %d", exchanges)
	}
	InvalidateQoderPATCache(pat)
}
