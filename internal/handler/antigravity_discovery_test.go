package handler

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// antigravityCatalogTransport answers :fetchAvailableModels with a canned body
// and 404s every other endpoint, so the catalog parser runs against a fixed
// upstream shape (the rich map response) without network access.
type antigravityCatalogTransport struct{ body string }

func (t antigravityCatalogTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if strings.HasSuffix(r.URL.Path, ":fetchAvailableModels") {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(t.body)),
			Request:    r,
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Request:    r,
	}, nil
}

// The synced catalog is the only source of media and chat inventory, so anything
// upstream marks disabled or internal — plus the IDE-only tab anchors — must not
// reach the cache: a disabled placeholder resolves to a 404 on call, and would
// otherwise be published through /v1/models and offered on the media page.
func TestFetchAntigravityCatalogDropsUnservableEntries(t *testing.T) {
	srv, _ := setupTestServer(t)
	body := `{"models":{
		"gemini-4-hydrogen-high": {"displayName": "Gemini 4 Hydrogen (High)", "apiProvider": "API_PROVIDER_GOOGLE_GEMINI", "maxTokens": 1048576, "maxOutputTokens": 262144, "thinkingLevel": 3, "supportsThinking": true, "supportsImages": true, "quotaInfo": {"remainingFraction": 0.99, "resetTime": "2026-11-14T06:28:03Z"}},
		"gemini-4-hydrogen-xhigh": {"displayName": "Gemini 4 Hydrogen (XHigh)", "apiProvider": "API_PROVIDER_GOOGLE_GEMINI", "maxTokens": 1048576, "maxOutputTokens": 262144, "thinkingLevel": 100, "supportsThinking": true, "supportsImages": true, "quotaInfo": {"remainingFraction": 0.99, "resetTime": "2026-11-14T06:28:03Z"}},
		"gemini-3.1-flash-image": {"displayName": "Gemini 3.1 Flash Image", "apiProvider": "API_PROVIDER_GOOGLE_GEMINI", "maxTokens": 1048576, "maxOutputTokens": 1024, "supportsImages": true},
		"claude-sonnet-5-5-high": {"displayName": "Claude Sonnet 5.5 (High)", "apiProvider": "API_PROVIDER_ANTHROPIC_VERTEX", "maxTokens": 1000000, "maxOutputTokens": 128000, "thinkingLevel": 3, "supportsThinking": true},
		"gemini-4-carbon-disabled": {"displayName": "Gemini 4 Carbon", "maxTokens": 1048576, "maxOutputTokens": 65536, "disabled": true},
		"gemini-internal-preview": {"displayName": "Gemini Internal Preview", "isInternal": true},
		"tab_flash_lite_preview": {"displayName": "Tab Flash Lite Preview", "maxTokens": 16384},
		"chat_20706": {"displayName": "Chat 20706"}
	}}`
	client := &http.Client{Transport: antigravityCatalogTransport{body: body}}

	got := srv.fetchAntigravityCatalog(context.Background(), client, "tok", "proj-1")

	ids := make([]string, 0, len(got))
	for _, m := range got {
		ids = append(ids, m.ID)
	}
	want := []string{"claude-sonnet-5-5-high", "gemini-3.1-flash-image", "gemini-4-hydrogen-high", "gemini-4-hydrogen-xhigh"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("catalog ids = %v, want %v", ids, want)
	}

	// The image engine must be distinguished from chat engines: that capability
	// split is what keeps it out of /v1/models and into the image tab.
	byID := map[string]model.ModelMetadata{}
	for _, m := range got {
		byID[m.ID] = m
	}
	if caps := byID["gemini-3.1-flash-image"].Capabilities; !slices.Contains(caps, "image-generation") || slices.Contains(caps, "chat") {
		t.Errorf("image engine capabilities = %v, want [image-generation]", caps)
	}
	if caps := byID["gemini-4-hydrogen-high"].Capabilities; !slices.Contains(caps, "chat") || !slices.Contains(caps, "reasoning") {
		t.Errorf("chat engine capabilities = %v, want chat and reasoning", caps)
	}
	// thinkingLevel is the depth upstream runs each id at; chat routing sends it
	// back instead of guessing from the tier suffix.
	if byID["gemini-4-hydrogen-xhigh"].ThinkingLevel != 100 {
		t.Errorf("xhigh thinking level = %d, want 100", byID["gemini-4-hydrogen-xhigh"].ThinkingLevel)
	}
	if byID["gemini-3.1-flash-image"].ThinkingLevel != 0 {
		t.Errorf("image engine declares no thinking level, got %d", byID["gemini-3.1-flash-image"].ThinkingLevel)
	}
	// The reset instant is what the quota park groups models by — models that reset
	// together drain on the same window — so it has to survive into the cache.
	if got := byID["gemini-4-hydrogen-high"].QuotaResetAt; got != "2026-11-14T06:28:03Z" {
		t.Errorf("hydrogen quota window reset = %q, want the upstream instant", got)
	}
	if byID["gemini-4-hydrogen-xhigh"].QuotaResetAt != byID["gemini-4-hydrogen-high"].QuotaResetAt {
		t.Errorf("tiers of one family must report one window")
	}
	if got := byID["gemini-3.1-flash-image"].QuotaResetAt; got != "" {
		t.Errorf("an entry upstream sends no quotaInfo for gained a window: %q", got)
	}
	// apiProvider states which API surface serves the model, which is what decides
	// whether Gemini-only request fields may be sent. It replaces reading the vendor
	// out of the model id, and no longer stamps a guessed family onto the row.
	if got := byID["claude-sonnet-5-5-high"].APIProvider; got != "API_PROVIDER_ANTHROPIC_VERTEX" {
		t.Errorf("claude row apiProvider = %q, want the upstream enum", got)
	}
	if got := byID["gemini-4-hydrogen-high"].APIProvider; got != "API_PROVIDER_GOOGLE_GEMINI" {
		t.Errorf("gemini row apiProvider = %q, want the upstream enum", got)
	}
	if got := byID["claude-sonnet-5-5-high"].Family; got != "" {
		t.Errorf("claude row gained an invented family %q, want it left for the metadata catalog", got)
	}
}
