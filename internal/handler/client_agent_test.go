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
	// The UA is parsed, not matched against a catalog: whatever product name the
	// client declares is what gets reported, version separated out. This is the
	// whole point -- a new CLI needs no gateway release to show up by name.
	cases := []struct {
		ua      string
		want    string
		version string
	}{
		{"claude-cli/2.0.59 (external, cli)", "claude-cli", "2.0.59"},
		{"codex_cli_rs/0.48.0 (Mac OS 15.0.0) x86_64", "codex_cli_rs", "0.48.0"},
		{"opencode/0.5.2", "opencode", "0.5.2"},
		{"Aider 0.74.1", "aider", "0.74.1"},
		{"Cursor/1.2.3", "cursor", "1.2.3"},
		{"Roo-Code/3.12", "roo-code", "3.12"},
		{"Anthropic/Python 0.42.0", "anthropic", "python"},
		{"OpenAI/Python 1.58.1", "openai", "python"},
		{"python-requests/2.32.3", "python-requests", "2.32.3"},
		{"Go-http-client/2.0", "go-http-client", "2.0"},
		{"PostmanRuntime/7.43.0", "postmanruntime", "7.43.0"},
		{"curl/8.9.1", "curl", "8.9.1"},
		// A tool the gateway has never heard of is reported by name rather than
		// disappearing into an "other" bucket that explains nothing.
		{"some-internal-runner/3", "some-internal-runner", "3"},
		{"guohongxing/omp.18.14.0", "guohongxing", "omp.18.14.0"},
		{"Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/131.0 Safari/537.36", "browser", ""},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		r.Header.Set("User-Agent", c.ua)
		if got := detectClientAgent(r); got != c.want {
			t.Errorf("UA %q => %q, want %q", c.ua, got, c.want)
		}
		if got := detectClientVersion(r); got != c.version {
			t.Errorf("UA %q version => %q, want %q", c.ua, got, c.version)
		}
	}

	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	if got := detectClientAgent(r); got != "unknown" {
		t.Errorf("empty UA => %q, want unknown", got)
	}
}

func TestParseUserAgentRejectsGarbage(t *testing.T) {
	for _, ua := range []string{"/1.2.3", "()", "   ", "//"} {
		if name, _ := parseUserAgent(ua); name != "" {
			t.Errorf("UA %q parsed to %q, want empty", ua, name)
		}
	}
	// Caller-controlled input must not bloat the grouping key.
	long := strings.Repeat("x", 500)
	if name, _ := parseUserAgent(long); len(name) > maxClientName {
		t.Errorf("name not capped: %d", len(name))
	}
}

// TestRecordUsagePersistsClient proves the fingerprint reaches both storage
// sinks (usageHistory.meta for the aggregate dimension, requestDetails for
// the detail row) through the shared recordUsage funnel.
func TestRecordUsagePersistsClient(t *testing.T) {
	srv, database := setupTestServer(t)
	uc := &usageContext{
		Provider:      "openai",
		Model:         "gpt-4o",
		ConnectionID:  "c1",
		APIKey:        "cg-test",
		Endpoint:      "/v1/chat/completions",
		Status:        200,
		Client:        detectClientAgent(newRequestWithUA("claude-cli/2.0.59 (external, cli)")),
		ClientVersion: detectClientVersion(newRequestWithUA("claude-cli/2.0.59 (external, cli)")),
		StartedAt:     time.Now().Add(-time.Millisecond),
	}
	srv.recordUsage(uc, usage.Usage{Known: true, PromptTokens: 5, CompletionTokens: 7})

	summary, err := database.GetUsageSummary(time.Time{}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if summary.ByClient["claude-cli"].Requests != 1 {
		t.Fatalf("ByClient must bucket claude-cli=1, got %+v", summary.ByClient)
	}
	if summary.ClientVersions["claude-cli"]["2.0.59"] != 1 {
		t.Fatalf("version not aggregated, got %+v", summary.ClientVersions)
	}

	res, err := database.GetRequestDetails(db.RequestDetailFilter{PageSize: 10})
	if err != nil || len(res.Details) == 0 {
		t.Fatalf("details missing: %v", err)
	}
	var row map[string]any
	if err := json.Unmarshal(res.Details[0], &row); err != nil {
		t.Fatal(err)
	}
	if row["client"] != "claude-cli" {
		t.Fatalf("detail row must carry client, got %s", strings.TrimSpace(string(res.Details[0])))
	}
	if row["clientVersion"] != "2.0.59" {
		t.Fatalf("detail row must carry clientVersion, got %v", row["clientVersion"])
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
