package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/usage"
)

func TestDetectClientAgent(t *testing.T) {
	cases := []struct {
		ua   string
		want string
	}{
		{"claude-cli/2.0.59 (external, cli)", "claude-code"},
		{"codex_cli_rs/0.48.0 (Mac OS 15.0.0) x86_64", "codex-cli"},
		{"opencode/0.5.2", "opencode"},
		{"Aider 0.74.1", "aider"},
		{"Cursor/1.2.3", "cursor"},
		{"Cline/3.0", "cline"},
		{"Roo-Code/3.12", "roocode"},
		{"anthropic/0.40.2", "anthropic-sdk"},
		{"Anthropic/Python 0.42.0", "anthropic-sdk"},
		{"OpenAI/Python 1.58.1", "openai-sdk"},
		{"google-genai/1.9.0", "gemini-sdk"},
		{"curl/8.9.1", "curl"},
		{"python-requests/2.32.3", "python-http"},
		{"python-httpx/0.28.1", "python-http"},
		{"Go-http-client/2.0", "go-http"},
		{"Bun/1.2.17", "bun"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36", "browser"},
		{"PostmanRuntime/7.43.0", "postman"},
		{"some-internal-runner/3", "other"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		r.Header.Set("User-Agent", c.ua)
		if got := detectClientAgent(r); got != c.want {
			t.Errorf("UA %q => %q, want %q", c.ua, got, c.want)
		}
	}
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if got := detectClientAgent(r); got != "unknown" {
		t.Errorf("empty UA => %q, want unknown", got)
	}
}

// TestRecordUsagePersistsClient proves the fingerprint reaches both storage
// sinks (usageHistory.meta for the aggregate dimension, requestDetails for
// the detail row) through the shared recordUsage funnel.
func TestRecordUsagePersistsClient(t *testing.T) {
	srv, database := setupTestServer(t)
	uc := &usageContext{
		Provider:     "openai",
		Model:        "gpt-4o",
		ConnectionID: "c1",
		APIKey:       "cg-test",
		Endpoint:     "/v1/chat/completions",
		Status:       200,
		Client:       detectClientAgent(newRequestWithUA("claude-cli/2.0.59 (external, cli)")),
		StartedAt:    time.Now().Add(-time.Millisecond),
	}
	srv.recordUsage(uc, usage.Usage{Known: true, PromptTokens: 5, CompletionTokens: 7})

	summary, err := database.GetUsageSummary(time.Time{}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if summary.ByClient["claude-code"].Requests != 1 {
		t.Fatalf("ByClient must bucket claude-code=1, got %+v", summary.ByClient)
	}

	res, err := database.GetRequestDetails(db.RequestDetailFilter{PageSize: 10})
	if err != nil || len(res.Details) == 0 {
		t.Fatalf("details missing: %v", err)
	}
	var row map[string]any
	if err := json.Unmarshal(res.Details[0], &row); err != nil {
		t.Fatal(err)
	}
	if row["client"] != "claude-code" {
		t.Fatalf("detail row must carry client, got %s", strings.TrimSpace(string(res.Details[0])))
	}
}

func newRequestWithUA(ua string) *http.Request {
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("User-Agent", ua)
	return r
}

func TestMediaRowsCarryClientFingerprint(t *testing.T) {
	srv, _, _ := mediaWalkFixture(t, http.StatusTooManyRequests)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/images/generations", strings.NewReader(`{"model":"openai/dall-e-3","prompt":"a cat"}`))
	req.Header.Set("User-Agent", "Aider 0.74.1")
	srv.Handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("fixture should succeed after one failover, got %d: %s", w.Code, w.Body.String())
	}

	summary, err := srv.DB.GetUsageSummary(time.Time{}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if summary.ByClient["aider"].Requests != 2 {
		t.Fatalf("failed attempt + success must both bucket aider: %+v", summary.ByClient)
	}

	res, err := srv.DB.GetRequestDetails(db.RequestDetailFilter{PageSize: 20})
	if err != nil || len(res.Details) < 2 {
		t.Fatalf("media rows missing: err=%v n=%d", err, len(res.Details))
	}
	nAider := 0
	for _, rawRow := range res.Details {
		var row map[string]any
		if err := json.Unmarshal(rawRow, &row); err != nil {
			t.Fatal(err)
		}
		if row["client"] == "aider" {
			nAider++
		}
	}
	if nAider != 2 {
		t.Fatalf("both media detail rows must fingerprint aider, got %d of %d", nAider, len(res.Details))
	}
}
