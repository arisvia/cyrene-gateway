package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

// seedSharedBareModel installs the same bare model id under two providers with
// different registry priorities: openai (30) sorts before opencode (40), so the
// shared name has an unambiguous first candidate. Writing opencode first keeps
// insertion order from being what decides anything.
func seedSharedBareModel(t *testing.T, database *db.DB) {
	t.Helper()
	for _, p := range []string{"opencode", "openai"} {
		conn := &model.ProviderConnection{
			ID:       "bare-" + p,
			Provider: p,
			AuthType: "api-key",
			IsActive: true,
			Data:     model.ConnectionData{APIKey: "sk-test"},
		}
		if err := database.CreateConnection(conn); err != nil {
			t.Fatalf("create connection %s: %v", p, err)
		}
		cached := `{"models":[{"id":"shared-reasoner","display_name":"Shared Reasoner","context_length":131072}]}`
		if err := database.KVSet(model.KVScopeProviderModelCache, p, cached); err != nil {
			t.Fatalf("seed cache %s: %v", p, err)
		}
	}
}

// TestBareRetrieveSkipsDeniedCandidates: a scoped key may call only one of the two
// providers publishing "shared-reasoner". Retrieve-by-bare-name used to 404 because
// the first candidate in list order was rejected outright instead of being passed
// over — so the model was callable (the router resolves bare names to the same
// order) but invisible to the tooling that reads its metadata first.
func TestBareRetrieveSkipsDeniedCandidates(t *testing.T) {
	srv, database := setupTestServer(t)
	seedSharedBareModel(t, database)

	get := func(path string, key *model.APIKey) *httptest.ResponseRecorder {
		req := newLocalRequest("GET", path, nil)
		if key != nil {
			req = req.WithContext(auth.WithAPIKey(req.Context(), key))
		}
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		return w
	}
	scoped := &model.APIKey{ID: "scoped", AllowedModels: []string{"opencode/shared-reasoner"}}

	w := get("/v1/models/shared-reasoner", scoped)
	if w.Code != http.StatusOK {
		t.Fatalf("bare retrieve: expected 200 from the allowed provider, got %d: %s", w.Code, w.Body.String())
	}
	var entry clientModelEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entry); err != nil {
		t.Fatalf("decode entry: %v (%s)", err, w.Body.String())
	}
	if entry.ID != "opencode/shared-reasoner" {
		t.Errorf("bare retrieve returned %q, want the only provider this key may call", entry.ID)
	}

	// The Gemini surface shares the builder, so it must agree.
	w = get("/v1beta/models/shared-reasoner", scoped)
	if w.Code != http.StatusOK {
		t.Fatalf("gemini bare retrieve: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var gstruct struct {
		Name     string `json:"name"`
		FullName string `json:"displayName"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &gstruct); err != nil {
		t.Fatalf("decode gemini model: %v (%s)", err, w.Body.String())
	}
	if !strings.HasSuffix(gstruct.Name, "opencode/shared-reasoner") {
		t.Errorf("gemini bare retrieve returned %q, want .../opencode/shared-reasoner", gstruct.Name)
	}

	// A qualified id the key may not use stays a 404 — skipping denied *candidates*
	// must not turn into a way to read models outside the allowlist.
	if w := get("/v1/models/openai/shared-reasoner", scoped); w.Code != http.StatusNotFound {
		t.Errorf("denied qualified id: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestStaticModelSeedKeepsFetchStamp: when a live fetch fails, the registry seed is
// cached so the panel is not blank — but writing it with a fresh timestamp made the
// 24h expiry renew itself on every 6h sync, so a provider with no working /models
// endpoint never looked stale again and its catalog froze silently.
func TestStaticModelSeedKeepsFetchStamp(t *testing.T) {
	srv, database := setupTestServer(t)
	prev := model.GetModelsDevCatalog()
	model.SetModelsDevCatalog(map[string]model.ModelsDevEntry{"seed-model": {Name: "Seed Model"}})
	t.Cleanup(func() { model.SetModelsDevCatalog(prev) })

	client := &http.Client{Timeout: 2 * time.Second}
	old := time.Now().UTC().Add(-30 * time.Hour)
	payload, err := json.Marshal(model.CachedModels{
		FetchedAt: old,
		Models:    []model.ModelMetadata{{ID: "seed-model", DisplayName: "Seed Model"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.KVSet(model.KVScopeProviderModelCache, "openai", string(payload)); err != nil {
		t.Fatal(err)
	}
	stored := func() model.CachedModels {
		t.Helper()
		raw, err := database.KVGet(model.KVScopeProviderModelCache, "openai")
		if err != nil {
			t.Fatalf("read back cache: %v", err)
		}
		var got model.CachedModels
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}

	seeds := []model.ModelMetadata{{ID: "seed-model", DisplayName: "Seed Model"}}
	if err := srv.cacheProviderModels("openai", seeds, client, false); err != nil {
		t.Fatalf("cache seed fallback: %v", err)
	}
	if got := stored(); !got.FetchedAt.Equal(old) {
		t.Errorf("registry-seed write moved the stamp to %v, want the previous %v", got.FetchedAt, old)
	}
	if _, stale := srv.cachedProviderModels("openai"); !stale {
		t.Error("a seeded fallback must still report stale so the next sync retries the network")
	}

	live := []model.ModelMetadata{{ID: "seed-model", DisplayName: "Seed Model"}}
	if err := srv.cacheProviderModels("openai", live, client, true); err != nil {
		t.Fatalf("cache live catalog: %v", err)
	}
	if got := stored(); got.FetchedAt.Equal(old) {
		t.Error("a real upstream catalog must restamp its fetch time")
	}
	if _, stale := srv.cachedProviderModels("openai"); stale {
		t.Error("a live catalog written now must not report stale")
	}
}

// TestGeminiErrorUsesGoogleEnvelope: the /v1beta surface is read by Google's SDKs,
// which parse error.code / error.message / error.status. This 500 is reachable —
// the connection query behind the listing can fail — and OpenAI's
// {"error":"internal error"} string arrives unreadable.
func TestGeminiErrorUsesGoogleEnvelope(t *testing.T) {
	srv, database := setupTestServer(t)
	seedSharedBareModel(t, database)
	// Break only the query the listing depends on. Closing the whole database
	// instead would fail the auth middleware first (503, by design), which is a
	// different path and never reaches the handler.
	if _, err := database.Conn().Exec(`ALTER TABLE providerConnections RENAME TO providerConnectionsGone`); err != nil {
		t.Fatalf("force the connection query to fail: %v", err)
	}

	for _, path := range []string{"/v1beta/models", "/v1beta/models/openai/shared-reasoner"} {
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, newLocalRequest("GET", path, nil))
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("%s: expected 500, got %d: %s", path, w.Code, w.Body.String())
		}
		var resp struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("%s: decode error envelope: %v (%s)", path, err, w.Body.String())
		}
		if resp.Error.Code != http.StatusInternalServerError || resp.Error.Status != "INTERNAL" || resp.Error.Message == "" {
			t.Errorf("%s: expected Google's error envelope, got %s", path, w.Body.String())
		}
	}
}

// TestClaimModelSyncRunsOncePerConnection: the model panel kicks a background
// re-fetch whenever it reads a stale cache, and a dashboard polling every few
// seconds stacked a fresh 30-second upstream call per request for the same
// connection while its credential was dead.
func TestClaimModelSyncRunsOncePerConnection(t *testing.T) {
	srv, _ := setupTestServer(t)

	release := make(chan struct{})
	var runs atomic.Int64
	blocking := func() {
		<-release
		runs.Add(1)
	}

	srv.claimModelSync("conn-a", blocking)
	srv.claimModelSync("conn-a", blocking) // already in flight: must be dropped
	srv.claimModelSync("conn-b", blocking) // different connection: independent

	close(release)
	srv.waitBackground()
	if got := runs.Load(); got != 2 {
		t.Fatalf("catalog refreshes ran %d times, want 2 (one per connection)", got)
	}

	// The claim is released when the work returns, so a later read can retry.
	srv.claimModelSync("conn-a", func() { runs.Add(10) })
	srv.waitBackground()
	if got := runs.Load(); got != 12 {
		t.Errorf("runs=%d after completion, want a released claim to accept work (12)", got)
	}
}
