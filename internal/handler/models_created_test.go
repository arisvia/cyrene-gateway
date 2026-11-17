package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

// seedCapabilityCatalog publishes one openai connection with a chat, an image and
// an embedding model, so a test can assert how each capability reaches every
// client-facing surface.
func seedCapabilityCatalog(t *testing.T, database *db.DB, connID string) {
	t.Helper()
	conn := &model.ProviderConnection{
		ID:       connID,
		Provider: "openai",
		AuthType: "api-key",
		IsActive: true,
		Data:     model.ConnectionData{APIKey: "sk-test"},
	}
	if err := database.CreateConnection(conn); err != nil {
		t.Fatalf("create connection: %v", err)
	}
	cached := `{"provider":"openai","models":[` +
		`{"id":"gpt-4o","display_name":"GPT-4o","context_length":128000,"max_output_tokens":16384,"capabilities":["chat"]},` +
		`{"id":"gpt-image-1","display_name":"GPT Image 1","capabilities":["chat","image-generation"]},` +
		`{"id":"text-embedding-3-small","display_name":"Text Embedding 3 Small","capabilities":["embeddings"]}]}`
	if err := database.KVSet(model.KVScopeProviderModelCache, "openai", cached); err != nil {
		t.Fatalf("seed catalog cache: %v", err)
	}
}

func getLocal(srv *Server, path string) *httptest.ResponseRecorder {
	req := newLocalRequest("GET", path, nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	return w
}

// TestModelsCreatedFromCatalog pins that the OpenAI `created` stamp comes from the
// catalog's release date and is omitted when there is no date — never fabricated.
func TestModelsCreatedFromCatalog(t *testing.T) {
	srv, database := setupTestServer(t)
	seedCapabilityCatalog(t, database, "created-conn")

	prev := model.GetModelsDevCatalog()
	model.SetModelsDevCatalog(map[string]model.ModelsDevEntry{
		"gpt-4o":  {Name: "GPT-4o", Context: 128000, Output: 16384, ReleaseDate: "2024-05-13"},
		"no-date": {Name: "No Date"},
	})
	t.Cleanup(func() { model.SetModelsDevCatalog(prev) })

	w := getLocal(srv, "/v1/models")
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []struct {
			ID      string `json:"id"`
			Created *int64 `json:"created"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	epoch := time.Date(2024, 5, 13, 0, 0, 0, 0, time.UTC).Unix()
	var found bool
	for _, m := range resp.Data {
		if m.ID == "openai/gpt-4o" {
			found = true
			if m.Created == nil || *m.Created != epoch {
				t.Errorf("created = %v, want %d (2024-05-13 UTC)", m.Created, epoch)
			}
			continue
		}
		// No catalog date means no stamp: publishing 0 or the fetch time would be
		// inventing a release date clients may sort or display.
		if m.Created != nil {
			t.Errorf("%s has no catalog release date but published created=%d", m.ID, *m.Created)
		}
	}
	if !found {
		t.Fatalf("gpt-4o missing from list")
	}

	// Retrieve must agree with the list, stamp included.
	rw := getLocal(srv, "/v1/models/openai/gpt-4o")
	var got clientModelEntry
	if err := json.Unmarshal(rw.Body.Bytes(), &got); err != nil {
		t.Fatalf("retrieve decode: %v", err)
	}
	if got.Created != epoch {
		t.Errorf("retrieve created = %d, want %d", got.Created, epoch)
	}
}

// TestProviderModelsCreatedFromCatalog pins the admin surface, which resolves the
// stamp on its own (buildModelItem → LookupCatalog) instead of reusing the client
// builder: the panel must show the same date /v1/models publishes, and must show
// nothing for a model the catalog has no date for.
func TestProviderModelsCreatedFromCatalog(t *testing.T) {
	srv, database := setupTestServer(t)
	seedCapabilityCatalog(t, database, "admin-created-conn")

	prev := model.GetModelsDevCatalog()
	model.SetModelsDevCatalog(map[string]model.ModelsDevEntry{
		"gpt-4o": {Name: "GPT-4o", Context: 128000, Output: 16384, ReleaseDate: "2024-05-13"},
	})
	t.Cleanup(func() { model.SetModelsDevCatalog(prev) })

	w := getLocal(srv, "/api/providers/admin-created-conn/models")
	if w.Code != http.StatusOK {
		t.Fatalf("admin models: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		RegistryModels []map[string]any `json:"registryModels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	epoch := time.Date(2024, 5, 13, 0, 0, 0, 0, time.UTC).Unix()
	var seen int
	for _, item := range resp.RegistryModels {
		id, _ := item["id"].(string)
		seen++
		raw, has := item["created"]
		if id == "gpt-4o" {
			if !has {
				t.Fatalf("gpt-4o: admin panel got no created key, want %d", epoch)
			}
			if got, _ := raw.(float64); int64(got) != epoch {
				t.Errorf("gpt-4o created = %v, want %d", raw, epoch)
			}
			continue
		}
		// omitempty must actually drop the field: a 0 here renders as a 1970 date.
		if has {
			t.Errorf("%s has no catalog release date but published created=%v", id, raw)
		}
	}
	if seen < 2 {
		t.Fatalf("registry rows = %d, want the seeded catalog to be listed", seen)
	}
}
