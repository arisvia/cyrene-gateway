package usage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/provider"
)

func TestFetchOpenCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/usage" {
			t.Errorf("expected path /usage, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer oc-test-key" {
			t.Errorf("expected Bearer oc-test-key, got %s", auth)
		}
		if client := r.Header.Get("x-opencode-client"); client != "desktop" {
			t.Errorf("expected x-opencode-client desktop, got %s", client)
		}
		// The quota call must carry the same client identity as chat: a bare
		// "opencode" or Go's default UA names the relay.
		if ua := r.Header.Get("User-Agent"); ua != provider.DefaultOpenCodeUA {
			t.Errorf("expected User-Agent %s, got %s", provider.DefaultOpenCodeUA, ua)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"usage": {
				"rolling": { "percent": 15.5, "resetsAt": "2026-09-10T18:00:00Z" },
				"weekly": { "percent": 42.0, "resetsAt": "2026-09-14T00:00:00Z" },
				"monthly": { "percent": 75.0, "resetsAt": "2026-10-01T00:00:00Z" }
			}
		}`))
	}))
	defer srv.Close()

	res := fetchOpenCode(context.Background(), srv.Client(), QuotaCredentials{
		Provider: "opencode",
		APIKey:   "oc-test-key",
		BaseURL:  srv.URL + "/chat/completions",
	})

	if res.Plan != "OpenCode Go" {
		t.Errorf("expected plan OpenCode Go, got %s", res.Plan)
	}
	if len(res.Quotas) != 3 {
		t.Fatalf("expected 3 quotas, got %d", len(res.Quotas))
	}

	rolling, ok := res.Quotas["Rolling"]
	if !ok {
		t.Fatal("expected Rolling quota")
	}
	if rolling.Used != 15.5 || rolling.Remaining != 84.5 {
		t.Errorf("unexpected rolling quota values: %+v", rolling)
	}
	if rolling.Unit != "%" {
		t.Errorf("expected unit %%, got %s", rolling.Unit)
	}
}

func TestFetchOpenCodeNoKey(t *testing.T) {
	res := fetchOpenCode(context.Background(), nil, QuotaCredentials{Provider: "opencode"})
	if res.Message == "" {
		t.Error("expected error message when API key is missing")
	}
}
