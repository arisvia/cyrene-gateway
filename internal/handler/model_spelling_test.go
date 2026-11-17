package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

// TestModelSpellingIsConsistentAcrossSurfaces pins one claim: a model string means
// the same thing to GET /v1/models, GET /v1/models/{id} and POST /v1/chat/completions.
// The reasoning-effort suffix and the context marker never name a different model,
// ids compare case-insensitively, and an allowlist pattern is evaluated against that
// same reduced spelling. Before this, a key scoped to the bare pattern "gpt-4o" could
// call openai/gpt-4o but could not see it in its own list, "OPENAI/*" refused a
// client that upper-cased the provider, and retrieve 404'd on "GPT-4O" for a model
// that completes fine — three answers out of one gateway.
func TestModelSpellingIsConsistentAcrossSurfaces(t *testing.T) {
	srv, database := setupTestServer(t)

	conn := &model.ProviderConnection{
		ID:       "spelling-conn",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		// A dead loopback route: the request must get all the way to the upstream
		// call for the assertion "not refused by the allowlist" to mean anything.
		Data: model.ConnectionData{APIKey: "sk-test", BaseURL: "http://127.0.0.1:9/v1"},
	}
	if err := database.CreateConnection(conn); err != nil {
		t.Fatalf("create connection: %v", err)
	}
	cached := `{"provider":"openai","models":[` +
		`{"id":"gpt-4o","display_name":"GPT-4o"},` +
		`{"id":"gpt-4o-mini","display_name":"GPT-4o mini"}]}`
	if err := database.KVSet(model.KVScopeProviderModelCache, "openai", cached); err != nil {
		t.Fatalf("seed catalog cache: %v", err)
	}

	listHas := func(key *model.APIKey, id string) bool {
		t.Helper()
		req := newLocalRequest("GET", "/v1/models", nil)
		if key != nil {
			req = req.WithContext(auth.WithAPIKey(req.Context(), key))
		}
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("list: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		for _, m := range resp.Data {
			if m.ID == id {
				return true
			}
		}
		return false
	}

	retrieve := func(key *model.APIKey, id string) int {
		t.Helper()
		req := newLocalRequest("GET", "/v1/models/"+id, nil)
		if key != nil {
			req = req.WithContext(auth.WithAPIKey(req.Context(), key))
		}
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		return w.Code
	}

	chat := func(key *model.APIKey, id string) int {
		t.Helper()
		body, err := json.Marshal(map[string]any{
			"model":      id,
			"max_tokens": 4,
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		req := newLocalRequest("POST", "/v1/chat/completions", bytes.NewReader(body))
		if key != nil {
			req = req.WithContext(auth.WithAPIKey(req.Context(), key))
		}
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		return w.Code
	}

	// allowed=false means the key is scoped elsewhere and every surface must agree
	// on refusing it: retrieve hides it (404, never 403, so existence is not leaked)
	// and the completion is rejected by the allowlist (403).
	cases := []struct {
		name     string
		key      *model.APIKey
		spelling string
		wantID   string
		allowed  bool
	}{
		{"no allowlist, exact id", nil, "openai/gpt-4o", "openai/gpt-4o", true},
		{"no allowlist, upper case", nil, "OPENAI/GPT-4O", "openai/gpt-4o", true},
		{"no allowlist, effort suffix", nil, "openai/gpt-4o(high)", "openai/gpt-4o", true},
		{"no allowlist, bare upper case", nil, "GPT-4O", "openai/gpt-4o", true},
		{"bare pattern covers the qualified id", &model.APIKey{ID: "bare", AllowedModels: []string{"gpt-4o"}}, "openai/gpt-4o", "openai/gpt-4o", true},
		{"bare pattern still hides its siblings", &model.APIKey{ID: "bare", AllowedModels: []string{"gpt-4o"}}, "openai/gpt-4o-mini", "openai/gpt-4o-mini", false},
		{"provider wildcard ignores case", &model.APIKey{ID: "wild", AllowedModels: []string{"OPENAI/*"}}, "openai/gpt-4o", "openai/gpt-4o", true},
		{"other provider is out of scope", &model.APIKey{ID: "wild", AllowedModels: []string{"anthropic/*"}}, "openai/gpt-4o", "openai/gpt-4o", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := listHas(tc.key, tc.wantID); got != tc.allowed {
				t.Errorf("list contains %q = %v, want %v", tc.wantID, got, tc.allowed)
			}
			if code := retrieve(tc.key, tc.spelling); code == http.StatusOK != tc.allowed {
				t.Errorf("retrieve(%q) = %d, want allowed=%v", tc.spelling, code, tc.allowed)
			}
			want := "pass"
			if !tc.allowed {
				want = "refuse"
			}
			if code := chat(tc.key, tc.spelling); (code == http.StatusForbidden) == tc.allowed {
				t.Errorf("chat(%q) = %d, want the allowlist to %s it", tc.spelling, code, want)
			}
		})
	}
}
