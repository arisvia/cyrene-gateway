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
		w.Write([]byte(`{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"42.5","granted_balance":"2.5","topped_up_balance":"40.0"},{"currency":"USD","total_balance":"10.0","granted_balance":"0","topped_up_balance":"10.0"}]}`))
	}))
	defer srv.Close()

	res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
		Provider: "deepseek", APIKey: "sk-ds", BaseURL: srv.URL,
	})
	if res.Plan != "DeepSeek" {
		t.Errorf("expected plan DeepSeek, got %q", res.Plan)
	}
	// One row per balance_infos entry, keyed by the upstream currency code.
	cny, ok := res.Quotas["CNY"]
	if !ok {
		t.Fatalf("expected a CNY row, got %v", res.Quotas)
	}
	if cny.Total != 42.5 || cny.Unit != "CNY" {
		t.Errorf("unexpected CNY balance: %+v", cny)
	}
	if !cny.Unlimited {
		t.Error("expected unlimited=true for positive balance")
	}
	if usd, ok := res.Quotas["USD"]; !ok || usd.Total != 10.0 {
		t.Errorf("a second currency lost its row: %+v", res.Quotas)
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
		w.Write([]byte(`{"data":{"level":"pro","limits":[
			{"type":"TOKENS_LIMIT","name":"Pro-Max","percentage":30,"nextResetTime":1893456000000},
			{"type":"TOKENS_LIMIT","percentage":80,"nextResetTime":1896048000000},
			{"type":"REQUEST_LIMIT","percentage":50}
		]}}`))
	}))
	defer srv.Close()

	res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
		Provider: "glm", APIKey: "sk-glm", BaseURL: srv.URL,
	})
	if res.Plan != "Pro" {
		t.Errorf("expected plan Pro, got %q", res.Plan)
	}
	// Every limit keeps its own row: keying them all "session" reported whichever
	// entry came last and hid the rest of the account.
	if len(res.Quotas) != 2 {
		t.Fatalf("expected the two TOKENS_LIMIT buckets, got %v", res.Quotas)
	}
	named, ok := res.Quotas["pro-max"]
	if !ok {
		t.Fatalf("expected the bucket named by upstream, got %v", res.Quotas)
	}
	if named.Used != 30 || named.Total != 100 || named.Remaining != 70 {
		t.Errorf("unexpected named quota: %+v", named)
	}
	if named.ResetAt == "" {
		t.Error("expected resetAt to be parsed")
	}
	unnamed, ok := res.Quotas["tokens_limit"]
	if !ok {
		t.Fatalf("expected a row keyed by the limit type, got %v", res.Quotas)
	}
	if unnamed.Used != 80 || unnamed.Remaining != 20 || unnamed.ResetAt == named.ResetAt {
		t.Errorf("unexpected unnamed quota: %+v", unnamed)
	}
}

func TestFetchMiniMax(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// count-based bucket: total 100, used 40 → remaining 60. The second entry is
		// the whole-series wildcard, which upstream spells "MiniMax-M*".
		w.Write([]byte(`{"base_resp":{"status_code":0,"status_msg":"success"},"model_remains":[{"model_name":"MiniMax-M1","current_interval_total_count":100,"current_interval_usage_count":40,"current_weekly_total_count":1000,"current_weekly_usage_count":200},{"model_name":"MiniMax-M*","current_interval_total_count":50,"current_interval_usage_count":10}]}`))
	}))
	defer srv.Close()

	res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
		Provider: "minimax", APIKey: "sk-mm", BaseURL: srv.URL,
	})
	// Window tokens name the upstream counter families; the response never states
	// how long an interval runs, so rows stopped claiming "(5h)"/"(7d)".
	q5, ok := res.Quotas["MiniMax-M1 (interval)"]
	if !ok {
		t.Fatalf("expected the interval bucket, got %v", res.Quotas)
	}
	if q5.Used != 40 || q5.Total != 100 || q5.Remaining != 60 {
		t.Errorf("unexpected interval quota: %+v", q5)
	}
	if q5.RemainingPercentage != 60 {
		t.Errorf("expected 60%% remaining, got %v", q5.RemainingPercentage)
	}
	q7, ok := res.Quotas["MiniMax-M1 (weekly)"]
	if !ok {
		t.Fatalf("expected the weekly bucket, got %v", res.Quotas)
	}
	if q7.Used != 200 || q7.Total != 1000 {
		t.Errorf("unexpected weekly quota: %+v", q7)
	}
	// The series wildcard reaches the panel as upstream wrote it.
	if star, ok := res.Quotas["MiniMax-M* (interval)"]; !ok || star.Total != 50 || star.Used != 10 {
		t.Errorf("wildcard model row = %+v (present: %t)", star, ok)
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
