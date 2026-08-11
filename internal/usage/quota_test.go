package usage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQuotaSupported(t *testing.T) {
	for _, p := range []string{"deepseek", "glm", "glm-cn", "minimax", "minimax-cn", "qoder"} {
		if !QuotaSupported(p) {
			t.Errorf("expected %q to be supported", p)
		}
	}
	if QuotaSupported("openai") {
		t.Error("openai should not have a real quota fetcher in Phase 31")
	}
}

func TestFetchQuotaUnsupported(t *testing.T) {
	res := FetchQuota(context.Background(), nil, QuotaCredentials{Provider: "openai"})
	if res.Message == "" {
		t.Error("expected informational message for unsupported provider")
	}
}

func TestFetchDeepseek(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/balance" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-ds" {
			t.Errorf("expected Bearer auth, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"42.5","granted_balance":"2.5","topped_up_balance":"40.0"}]}`))
	}))
	defer srv.Close()

	res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
		Provider: "deepseek", APIKey: "sk-ds", BaseURL: srv.URL,
	})
	if res.Plan != "DeepSeek" {
		t.Errorf("expected plan DeepSeek, got %q", res.Plan)
	}
	q, ok := res.Quotas["Balance (CNY)"]
	if !ok {
		t.Fatalf("expected Balance (CNY) quota, got %v", res.Quotas)
	}
	if q.Total != 42.5 {
		t.Errorf("expected total 42.5, got %v", q.Total)
	}
	if !q.Unlimited {
		t.Error("expected unlimited=true for positive balance")
	}
}

func TestFetchDeepseekAuthFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
		Provider: "deepseek", APIKey: "bad", BaseURL: srv.URL,
	})
	if res.Message == "" {
		t.Error("expected auth failure message")
	}
}

func TestFetchGLM(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/monitor/usage/quota/limit" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"level":"pro","limits":[{"type":"TOKENS_LIMIT","percentage":30,"nextResetTime":1893456000000}]}}`))
	}))
	defer srv.Close()

	res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
		Provider: "glm", APIKey: "sk-glm", BaseURL: srv.URL,
	})
	if res.Plan != "Pro" {
		t.Errorf("expected plan Pro, got %q", res.Plan)
	}
	q, ok := res.Quotas["session"]
	if !ok {
		t.Fatalf("expected session quota, got %v", res.Quotas)
	}
	if q.Used != 30 || q.Total != 100 || q.Remaining != 70 {
		t.Errorf("unexpected quota values: %+v", q)
	}
	if q.ResetAt == "" {
		t.Error("expected resetAt to be parsed")
	}
}

func TestFetchMiniMax(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// count-based bucket: total 100, used 40 → remaining 60.
		w.Write([]byte(`{"base_resp":{"status_code":0,"status_msg":"success"},"model_remains":[{"model_name":"MiniMax-M1","current_interval_total_count":100,"current_interval_usage_count":40,"current_weekly_total_count":1000,"current_weekly_usage_count":200}]}`))
	}))
	defer srv.Close()

	res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
		Provider: "minimax", APIKey: "sk-mm", BaseURL: srv.URL,
	})
	q5, ok := res.Quotas["MiniMax-M1 (5h)"]
	if !ok {
		t.Fatalf("expected 5h quota, got %v", res.Quotas)
	}
	if q5.Used != 40 || q5.Total != 100 || q5.Remaining != 60 {
		t.Errorf("unexpected 5h quota: %+v", q5)
	}
	if q5.RemainingPercentage != 60 {
		t.Errorf("expected 60%% remaining, got %v", q5.RemainingPercentage)
	}
	q7, ok := res.Quotas["MiniMax-M1 (7d)"]
	if !ok {
		t.Fatalf("expected 7d quota, got %v", res.Quotas)
	}
	if q7.Used != 200 || q7.Total != 1000 {
		t.Errorf("unexpected 7d quota: %+v", q7)
	}
}

func TestFetchMiniMaxAuthInvalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"base_resp":{"status_code":1004,"status_msg":"invalid api key"}}`))
	}))
	defer srv.Close()

	res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
		Provider: "minimax", APIKey: "bad", BaseURL: srv.URL,
	})
	if res.Message == "" {
		t.Error("expected invalid-key message")
	}
}

func TestParseResetTime(t *testing.T) {
	// Millisecond timestamp.
	if got := parseResetTime(float64(1893456000000)); got == "" {
		t.Error("expected non-empty ISO for ms timestamp")
	}
	// Second timestamp (< 1e12) is scaled up.
	if got := parseResetTime(float64(1893456000)); got == "" {
		t.Error("expected non-empty ISO for s timestamp")
	}
	// Numeric string.
	if got := parseResetTime("1893456000000"); got == "" {
		t.Error("expected non-empty ISO for numeric string")
	}
	// Zero / empty → "".
	if got := parseResetTime(float64(0)); got != "" {
		t.Errorf("expected empty for 0, got %q", got)
	}
}
