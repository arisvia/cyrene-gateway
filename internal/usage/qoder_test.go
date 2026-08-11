package usage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Device/job tokens hit the quota endpoint directly (no PAT exchange).
func TestFetchQoder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/quota/usage" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer jt-test-token" {
			t.Errorf("expected Bearer jt-test-token, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"userQuota": {"total": 2000, "used": 500, "remaining": 1500, "unit": "credits"},
			"orgResourcePackage": {"total": 0, "used": 0, "remaining": 0},
			"totalUsagePercentage": 25,
			"isQuotaExceeded": false,
			"expiresAt": 1781594470000
		}`))
	}))
	defer srv.Close()

	res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
		Provider: "qoder", AccessToken: "jt-test-token", BaseURL: srv.URL,
	})
	if !strings.Contains(res.Plan, "25%") {
		t.Errorf("expected plan with 25%% usage, got %q", res.Plan)
	}
	q, ok := res.Quotas["user"]
	if !ok {
		t.Fatalf("expected user quota, got %v", res.Quotas)
	}
	if q.Used != 500 || q.Total != 2000 || q.Remaining != 1500 {
		t.Errorf("unexpected quota values: %+v", q)
	}
	if q.ResetAt == "" {
		t.Error("expected resetAt from expiresAt")
	}
	if _, ok := res.Quotas["organization"]; ok {
		t.Error("zero-value org quota should be omitted")
	}
}

func TestFetchQoderNoToken(t *testing.T) {
	res := FetchQuota(context.Background(), nil, QuotaCredentials{Provider: "qoder"})
	if !strings.Contains(res.Message, "no access token") {
		t.Errorf("expected no-token message, got %q", res.Message)
	}
}
