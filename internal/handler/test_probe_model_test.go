package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func seedProbeCatalog(t *testing.T, srv *Server, providerID string, models ...model.ModelMetadata) {
	t.Helper()
	raw, err := json.Marshal(model.CachedModels{Models: models})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.DB.KVSet(model.KVScopeProviderModelCache, providerID, string(raw)); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
}

// The connectivity probe must name a model the account really has. Hard-naming one
// vendor id per API shape made a healthy credential look broken the day upstream
// retired it, or when the plan never carried that model.
func TestProbeModelSelectsFromCatalog(t *testing.T) {
	srv, database := setupTestServer(t)
	conn := &model.ProviderConnection{
		ID: "probe-1", Provider: "anthropic", AuthType: "api-key", IsActive: true,
		Data: model.ConnectionData{APIKey: "k"},
	}

	if got := srv.probeModel(conn); got != "" {
		t.Errorf("nothing is known about this account yet, got %q", got)
	}

	seedProbeCatalog(t, srv, "anthropic",
		// The image entry does not take chat contents; the reasoning entry can spend
		// the probe's whole budget on thinking and answer empty; the disabled entry is
		// not ours to send traffic to.
		model.ModelMetadata{ID: "claude-vision-x", Capabilities: []string{"image-generation", "vision"}},
		model.ModelMetadata{ID: "claude-opus-think", Capabilities: []string{"chat", "reasoning"}},
		model.ModelMetadata{ID: "claude-haiku-plain", Capabilities: []string{"chat"}},
		model.ModelMetadata{ID: "claude-old-haiku", Capabilities: []string{"chat"}},
	)
	if err := database.KVSet(model.KVScopeDisabledModels, "anthropic/claude-old-haiku", "true"); err != nil {
		t.Fatalf("disable model: %v", err)
	}
	if got := srv.probeModel(conn); got != "claude-haiku-plain" {
		t.Errorf("probe model = %q, want the enabled plain chat entry", got)
	}

	// Reasoning is avoided only while something cheaper exists.
	seedProbeCatalog(t, srv, "anthropic",
		model.ModelMetadata{ID: "claude-vision-x", Capabilities: []string{"image-generation"}},
		model.ModelMetadata{ID: "claude-opus-think", Capabilities: []string{"chat", "reasoning"}},
	)
	if got := srv.probeModel(conn); got != "claude-opus-think" {
		t.Errorf("probe model = %q, want the only chat-capable entry even if it reasons", got)
	}

	// Every chat entry disabled: report nothing rather than probe with a blocked id.
	seedProbeCatalog(t, srv, "anthropic",
		model.ModelMetadata{ID: "claude-opus-think", Capabilities: []string{"chat", "reasoning"}},
	)
	if err := database.KVSet(model.KVScopeDisabledModels, "anthropic/claude-opus-think", "true"); err != nil {
		t.Fatalf("disable model: %v", err)
	}
	if got := srv.probeModel(conn); got != "" {
		t.Errorf("probe model = %q, want empty when every entry is disabled", got)
	}

	// A connection's own custom models count even when no catalog was synced.
	custom, err := json.Marshal([]customModel{{ID: "private-llm-7b"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.KVSet(customModelsScope, conn.ID, string(custom)); err != nil {
		t.Fatalf("seed custom models: %v", err)
	}
	if got := srv.probeModel(conn); got != "private-llm-7b" {
		t.Errorf("probe model = %q, want the connection's custom model", got)
	}
}

// The chosen id is what actually reaches upstream: a stale literal in the probe body
// would otherwise keep failing accounts that are fine.
func TestConnectionProbeSendsCatalogModel(t *testing.T) {
	srv, _ := setupTestServer(t)
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel = body.Model
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"content": []any{map[string]any{"type": "text", "text": "ok"}}})
	}))
	defer upstream.Close()

	conn := &model.ProviderConnection{
		ID: "probe-live", Provider: "anthropic", AuthType: "api-key", IsActive: true,
		Data: model.ConnectionData{APIKey: "k", BaseURL: upstream.URL + "/v1/messages"},
	}
	seedProbeCatalog(t, srv, "anthropic",
		model.ModelMetadata{ID: "claude-account-model", Capabilities: []string{"chat"}},
	)

	res := srv.testConnection(httptest.NewRequest(http.MethodPost, "/test", nil), conn, srv.getHTTPClient(5*time.Second))
	if !res.OK {
		t.Fatalf("probe should succeed against the mock upstream: %+v", res)
	}
	if gotModel != "claude-account-model" {
		t.Errorf("upstream was asked for %q, want the catalog model", gotModel)
	}
}
