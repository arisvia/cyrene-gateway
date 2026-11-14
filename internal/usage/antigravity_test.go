package usage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func modelsFixture(t *testing.T, raw string) AntigravityModelsResponse {
	t.Helper()
	var resp AntigravityModelsResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("models fixture: %v", err)
	}
	return resp
}

func summaryFixture(t *testing.T, raw string) AntigravityQuotaSummaryResponse {
	t.Helper()
	var summary AntigravityQuotaSummaryResponse
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		t.Fatalf("summary fixture: %v", err)
	}
	return summary
}

// fetchAvailableModels (the fallback endpoint) reports one quota window per model
// and never names the pool it belongs to, so rows carry the upstream model id and
// the upstream display name. Inventing "gemini"/"claude"/"gemini4" aggregates hid
// which model drained and published labels upstream never uses.
func TestClusterAntigravityQuotas(t *testing.T) {
	resp := modelsFixture(t, `{"models":{
		"gemini-3.8-flash-high":    {"displayName":"Gemini 3.8 Flash (High)","quotaInfo":{"remainingFraction":0.95,"resetTime":"2099-09-10T14:21:00Z"}},
		"claude-sonnet-4-6":        {"displayName":"Claude Sonnet 4.6 (Thinking)","quotaInfo":{"remainingFraction":1.0,"resetTime":"2099-09-10T15:00:00Z"}},
		"gpt-oss-120b-medium":      {"quotaInfo":{"remainingFraction":0.5,"resetTime":"2099-09-10T15:00:00Z"}},
		"imagen-3.0-generate":      {"displayName":"Imagen 3.0","quotaInfo":{"remainingFraction":0.0,"isExhausted":true}},
		"gemini-4-carbon-disabled": {"displayName":"Gemini 4 Carbon"},
		"chat_experimental_internal": {"displayName":"Internal Test","quotaInfo":{"remainingFraction":0.95}},
		"tab_flash_lite_preview":   {"displayName":"Tab Anchor","quotaInfo":{"remainingFraction":0.95}}
	}}`)

	quotas := clusterAntigravityQuotas(resp)

	if len(quotas) != 4 {
		t.Fatalf("expected one row per servable model, got %d: %+v", len(quotas), quotas)
	}
	gemini := quotas["gemini-3.8-flash-high"]
	if gemini.DisplayName != "Gemini 3.8 Flash (High)" || gemini.RemainingPercentage != 95.0 {
		t.Errorf("gemini row = %+v", gemini)
	}
	if gemini.ResetAt != "2099-09-10T14:21:00Z" || gemini.ResetSoon {
		t.Errorf("gemini reset = %q soon=%t", gemini.ResetAt, gemini.ResetSoon)
	}
	// A model upstream gives no display name still keys and labels by its id.
	if oss := quotas["gpt-oss-120b-medium"]; oss.DisplayName != "gpt-oss-120b-medium" || oss.RemainingPercentage != 50.0 {
		t.Errorf("unnamed model row = %+v", oss)
	}
	// isExhausted overrides the fraction, matching what a call would experience.
	if img := quotas["imagen-3.0-generate"]; img.RemainingPercentage != 0.0 {
		t.Errorf("exhausted image row = %+v", img)
	}
	// Google returns a fraction and a reset time, never a request count: nothing
	// absolute may be fabricated from it.
	for key, q := range quotas {
		if q.Total != 0 || q.Used != 0 || q.Remaining != 0 {
			t.Errorf("%s publishes an absolute total upstream never reports: %+v", key, q)
		}
	}
	for _, absent := range []string{"gemini", "gemini4", "claude", "chat_experimental_internal", "tab_flash_lite_preview", "gemini-4-carbon-disabled"} {
		if _, ok := quotas[absent]; ok {
			t.Errorf("%q must not appear as a quota row: %+v", absent, quotas)
		}
	}
}

func TestClusterAntigravityQuotas_ResetTimeNormalization(t *testing.T) {
	future := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	resp := modelsFixture(t, `{"models":{
		"gemini-sentinel": {"quotaInfo":{"remainingFraction":0.5,"resetTime":"即将重置"}},
		"gemini-future":   {"quotaInfo":{"remainingFraction":0.5,"resetTime":"`+future+`"}},
		"gemini-stale":    {"quotaInfo":{"remainingFraction":0.5,"resetTime":"2020-01-01T00:00:00Z"}},
		"gemini-garbage":  {"quotaInfo":{"remainingFraction":0.5,"resetTime":"not-a-valid-time"}}
	}}`)

	quotas := clusterAntigravityQuotas(resp)

	sentinel := quotas["gemini-sentinel"]
	if !sentinel.ResetSoon || sentinel.ResetAt != "" {
		t.Errorf("sentinel reset = %+v", sentinel)
	}
	futureRow := quotas["gemini-future"]
	if futureRow.ResetSoon || futureRow.ResetAt == "" {
		t.Errorf("future reset = %+v", futureRow)
	}
	// A stamp already in the past is reported as imminent: selection ignores
	// past-deadline buckets rather than pinning traffic to a stale snapshot.
	stale := quotas["gemini-stale"]
	if !stale.ResetSoon {
		t.Errorf("stale reset = %+v", stale)
	}
	garbage := quotas["gemini-garbage"]
	if garbage.ResetAt != "" || garbage.ResetSoon {
		t.Errorf("garbage reset = %+v", garbage)
	}
	for key, q := range quotas {
		if q.RemainingPercentage != 50.0 {
			t.Errorf("%s percentage = %v, want 50", key, q.RemainingPercentage)
		}
	}
}

func TestFetchAntigravity_MockServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1internal:fetchAvailableModels" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"models": {
				"gemini-3.8-flash-high": {
					"displayName": "Gemini 3.8 Flash",
					"quotaInfo": {"remainingFraction": 0.8, "resetTime": "2099-09-10T18:00:00Z"}
				},
				"claude-sonnet-4-6": {
					"displayName": "Claude Sonnet 4.6",
					"quotaInfo": {"remainingFraction": 1.0, "resetTime": "2099-09-10T19:00:00Z"}
				}
			}
		}`))
	}))
	defer ts.Close()

	res := FetchQuota(context.Background(), ts.Client(), QuotaCredentials{
		Provider:    "antigravity",
		AccessToken: "mock-token",
		BaseURL:     ts.URL,
	})

	if len(res.Quotas) != 2 {
		t.Fatalf("expected 2 quotas, got %d: %+v", len(res.Quotas), res.Quotas)
	}
	if res.Quotas["gemini-3.8-flash-high"].DisplayName != "Gemini 3.8 Flash" {
		t.Errorf("unexpected gemini label: %+v", res.Quotas["gemini-3.8-flash-high"])
	}
	if res.Quotas["claude-sonnet-4-6"].DisplayName != "Claude Sonnet 4.6" {
		t.Errorf("unexpected claude label: %+v", res.Quotas["claude-sonnet-4-6"])
	}
}

// retrieveUserQuotaSummary is the authoritative pool view: it hands back its own
// bucket ids and window tokens, so the panel keys by those ids and labels with the
// upstream group name. Keyword-matching the group name to synthesize
// "gemini_session"/"Claude & GPT (5h Rolling)" replaced upstream text with strings
// that had to be maintained by hand.
func TestClusterAntigravitySummary(t *testing.T) {
	summary := summaryFixture(t, `{"groups":[
		{"displayName":"Gemini Models","buckets":[
			{"bucketId":"gemini-5h","window":"5h","displayName":"Five Hour Limit Remaining","remainingFraction":0.8},
			{"bucketId":"gemini-weekly","window":"weekly","displayName":"Weekly Limit Remaining","remainingFraction":0.6}
		]},
		{"displayName":"Claude and GPT models","buckets":[
			{"bucketId":"3p-5h","window":"5h","remainingFraction":0.4},
			{"bucketId":"3p-weekly","window":"weekly","remainingFraction":0.6}
		]}
	]}`)

	quotas := clusterAntigravitySummary(summary)
	if len(quotas) != 4 {
		t.Fatalf("expected one row per upstream bucket, got %d: %+v", len(quotas), quotas)
	}

	cases := map[string]struct {
		display string
		pct     float64
	}{
		"gemini-5h":     {"Gemini Models · 5h", 80.0},
		"gemini-weekly": {"Gemini Models · weekly", 60.0},
		"3p-5h":         {"Claude and GPT models · 5h", 40.0},
		"3p-weekly":     {"Claude and GPT models · weekly", 60.0},
	}
	for key, want := range cases {
		q, ok := quotas[key]
		if !ok {
			t.Fatalf("bucket %q missing: %+v", key, quotas)
		}
		if q.DisplayName != want.display || q.RemainingPercentage != want.pct {
			t.Errorf("bucket %q = %+v, want label %q at %v%%", key, q, want.display, want.pct)
		}
		if q.Total != 0 || q.Remaining != 0 {
			t.Errorf("bucket %q fabricates an absolute total: %+v", key, q)
		}
	}
	for _, synthesized := range []string{"gemini_session", "gemini_weekly", "claude_session", "gemini4"} {
		if _, ok := quotas[synthesized]; ok {
			t.Errorf("synthesized key %q survived: %+v", synthesized, quotas)
		}
	}
}

// Argon is a separate pool because upstream publishes it as a separate bucket id,
// not because a name matched "gemini-4": grouping by upstream identity keeps a new
// pool from being folded into an existing row.
func TestClusterAntigravitySummaryKeepsArgonPoolSeparate(t *testing.T) {
	summary := summaryFixture(t, `{"groups":[
		{"displayName":"Gemini Models","buckets":[{"bucketId":"gemini-5h","window":"5h","remainingFraction":1.0,"resetTime":"2099-11-14T06:28:03Z"}]},
		{"displayName":"Gemini 4 Argon","buckets":[{"bucketId":"gemini-argon-wtu-weekly","window":"weekly","remainingFraction":0.42,"resetTime":"2099-11-21T01:28:03Z"}]}
	]}`)

	quotas := clusterAntigravitySummary(summary)
	gemini, ok := quotas["gemini-5h"]
	if !ok || gemini.RemainingPercentage != 100.0 {
		t.Fatalf("gemini bucket lost: %+v", quotas)
	}
	if gemini.ResetAt != "2099-11-14T06:28:03Z" || gemini.ResetSoon {
		t.Errorf("gemini reset = %+v", gemini)
	}
	argon, ok := quotas["gemini-argon-wtu-weekly"]
	if !ok {
		t.Fatalf("argon bucket lost or collided with the gemini row: %+v", quotas)
	}
	if argon.RemainingPercentage != 42.0 || argon.DisplayName != "Gemini 4 Argon · weekly" {
		t.Errorf("argon bucket = %+v", argon)
	}
}

// Google reuses window ids across plan tiers on some accounts; a duplicate must
// not silently overwrite the row already on screen.
func TestClusterAntigravitySummaryDedupesRepeatedBucketID(t *testing.T) {
	summary := summaryFixture(t, `{"groups":[
		{"displayName":"Gemini Models","buckets":[{"bucketId":"shared-window","window":"5h","remainingFraction":0.9}]},
		{"displayName":"Claude and GPT models","buckets":[{"bucketId":"shared-window","window":"5h","remainingFraction":0.2}]}
	]}`)

	quotas := clusterAntigravitySummary(summary)
	if len(quotas) != 2 {
		t.Fatalf("duplicate bucket id collapsed a row: %+v", quotas)
	}
	if _, ok := quotas["shared-window"]; !ok {
		t.Errorf("first occurrence lost: %+v", quotas)
	}
	if second, ok := quotas["shared-window-2"]; !ok || second.RemainingPercentage != 20.0 {
		t.Errorf("second occurrence = %+v (ok=%t)", second, ok)
	}
}

// A bucket without a group name still needs a readable label; the window token and
// then the bucket id are the fallbacks.
func TestClusterAntigravitySummaryLabelFallsBackUpstreamFields(t *testing.T) {
	summary := summaryFixture(t, `{"groups":[
		{"buckets":[{"bucketId":"bare-window","window":"weekly","remainingFraction":1.0}]},
		{"buckets":[{"bucketId":"unnamed-group-2","remainingFraction":1.0,"displayName":"Weekly Limit Remaining"}]},
		{"buckets":[{"bucketId":"nameless","remainingFraction":1.0}]}
	]}`)

	quotas := clusterAntigravitySummary(summary)
	if got := quotas["bare-window"].DisplayName; got != "bare-window · weekly" {
		t.Errorf("window-only label = %q", got)
	}
	if got := quotas["unnamed-group-2"].DisplayName; got != "Weekly Limit Remaining" {
		t.Errorf("bucket-name label = %q", got)
	}
	if got := quotas["nameless"].DisplayName; got != "nameless" {
		t.Errorf("id-only label = %q", got)
	}
}

// The summary endpoint is the primary source: while it answers, the panel shows
// upstream pool buckets instead of per-model rows.
func TestFetchAntigravity_PrefersQuotaSummary(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1internal:retrieveUserQuotaSummary":
			w.Write([]byte(`{"groups":[{"displayName":"Gemini Models","buckets":[
				{"bucketId":"gemini-5h","window":"5h","remainingFraction":0.75,"resetTime":"2099-09-10T18:00:00Z"}]}]}`))
		case "/v1internal:fetchAvailableModels":
			w.Write([]byte(`{"models":{"gemini-3.8-flash-high":{"displayName":"Gemini 3.8 Flash","quotaInfo":{"remainingFraction":0.75,"resetTime":"2099-09-10T18:00:00Z"}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	res := FetchQuota(context.Background(), ts.Client(), QuotaCredentials{
		Provider:    "antigravity",
		AccessToken: "mock-token",
		BaseURL:     ts.URL,
	})

	if len(res.Quotas) != 1 {
		t.Fatalf("expected the single upstream bucket, got %+v", res.Quotas)
	}
	q, ok := res.Quotas["gemini-5h"]
	if !ok {
		t.Fatalf("row is not keyed by the upstream bucket id: %+v", res.Quotas)
	}
	if q.DisplayName != "Gemini Models · 5h" || q.RemainingPercentage != 75.0 {
		t.Errorf("bucket row = %+v", q)
	}
}

func TestFetchAntigravity_MissingToken(t *testing.T) {
	res := FetchQuota(context.Background(), http.DefaultClient, QuotaCredentials{Provider: "antigravity"})
	if res.Message == "" {
		t.Errorf("expected failure message for missing token, got %+v", res)
	}
}
