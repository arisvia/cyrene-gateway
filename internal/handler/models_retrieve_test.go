package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

// TestModelRetrieve pins GET /v1/models/{id}: the same SDKs that list models also
// retrieve one by name, and a second definition of the same model is how a gateway
// starts lying to its clients. So retrieve must return byte-identical fields to the
// list, must resolve the bare form clients actually type, and must not leak models a
// scoped key cannot call.
func TestModelRetrieve(t *testing.T) {
	srv, database := setupTestServer(t)

	conn := &model.ProviderConnection{
		ID:       "retrieve-conn",
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data:     model.ConnectionData{APIKey: "sk-test"},
	}
	if err := database.CreateConnection(conn); err != nil {
		t.Fatalf("create connection: %v", err)
	}
	cached := `{"provider":"openai","models":[` +
		`{"id":"gpt-4o","display_name":"GPT-4o","context_length":128000,"max_output_tokens":16384},` +
		`{"id":"gpt-image-1","display_name":"GPT Image 1","capabilities":["image-generation"]}]}`
	if err := database.KVSet(model.KVScopeProviderModelCache, "openai", cached); err != nil {
		t.Fatalf("seed catalog cache: %v", err)
	}
	if err := database.KVSet(model.KVScopeDisabledModels, "openai/gpt-hidden", "true"); err != nil {
		t.Fatalf("disable model: %v", err)
	}

	get := func(path string, key *model.APIKey) *httptest.ResponseRecorder {
		req := newLocalRequest("GET", path, nil)
		if key != nil {
			req = req.WithContext(auth.WithAPIKey(req.Context(), key))
		}
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		return w
	}

	entryOf := func(t *testing.T, w *httptest.ResponseRecorder) clientModelEntry {
		t.Helper()
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var e clientModelEntry
		if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
			t.Fatalf("decode entry: %v (%s)", err, w.Body.String())
		}
		return e
	}

	listEntryOf := func(t *testing.T, path, id string) clientModelEntry {
		t.Helper()
		w := get(path, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("list %s: expected 200, got %d: %s", path, w.Code, w.Body.String())
		}
		var resp struct {
			Data []clientModelEntry `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		for _, m := range resp.Data {
			if m.ID == id {
				return m
			}
		}
		t.Fatalf("%s not found in list %s", id, path)
		return clientModelEntry{}
	}

	t.Run("mirrors the list entry exactly", func(t *testing.T) {
		want := listEntryOf(t, "/v1/models", "openai/gpt-4o")
		got := entryOf(t, get("/v1/models/openai/gpt-4o", nil))
		if !reflect.DeepEqual(want, got) {
			t.Errorf("retrieve = %+v, want the list entry %+v", got, want)
		}
		if got.Object != "model" || got.Name != "GPT-4o" || got.DisplayName != "GPT-4o" ||
			got.ContextLength != 128000 || got.MaxOutput != 16384 {
			t.Errorf("retrieve fields wrong: %+v", got)
		}
	})

	t.Run("resolves the bare model id", func(t *testing.T) {
		got := entryOf(t, get("/v1/models/gpt-4o", nil))
		if got.ID != "openai/gpt-4o" {
			t.Errorf("bare id resolved to %q, want openai/gpt-4o", got.ID)
		}
	})

	t.Run("reaches an image model the default list hides", func(t *testing.T) {
		// The chat surface filters image generators out; retrieving one by name
		// must still work, otherwise the id is listed nowhere and fetched nowhere.
		if _, err := json.Marshal(listEntryOf(t, "/v1/models?type=image", "openai/gpt-image-1")); err != nil {
			t.Fatal(err)
		}
		got := entryOf(t, get("/v1/models/openai/gpt-image-1", nil))
		if !reflect.DeepEqual(got, listEntryOf(t, "/v1/models?type=image", "openai/gpt-image-1")) {
			t.Errorf("image retrieve mismatch: %+v", got)
		}
	})

	t.Run("scoped keys cannot read models outside their allowlist", func(t *testing.T) {
		scoped := &model.APIKey{ID: "scoped", AllowedModels: []string{"openai/gpt-4o"}}
		if w := get("/v1/models/openai/gpt-4o", scoped); w.Code != http.StatusOK {
			t.Errorf("allowed model: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if w := get("/v1/models/openai/gpt-image-1", scoped); w.Code != http.StatusNotFound {
			t.Errorf("scoped-out model: expected 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown and disabled ids are 404 model_not_found", func(t *testing.T) {
		for _, path := range []string{"/v1/models/openai/nope", "/v1/models/openai/gpt-hidden", "/v1/models/nope"} {
			w := get(path, nil)
			if w.Code != http.StatusNotFound {
				t.Errorf("%s: expected 404, got %d: %s", path, w.Code, w.Body.String())
				continue
			}
			var resp struct {
				Error struct {
					Code    string `json:"code"`
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("%s: decode error body: %v", path, err)
			}
			if resp.Error.Code != "model_not_found" || resp.Error.Type != "invalid_request_error" {
				t.Errorf("%s: unexpected error shape %+v", path, resp.Error)
			}
		}
	})
}
