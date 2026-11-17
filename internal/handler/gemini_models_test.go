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

// TestGeminiModelListing covers Google's model listing schema on /v1beta: the
// google-genai SDK lists there, then retrieves and generates using the returned
// resource name. Every one of those steps must agree with what /v1 publishes.
func TestGeminiModelListing(t *testing.T) {
	srv, database := setupTestServer(t)
	seedCapabilityCatalog(t, database, "gemini-list-conn")

	get := func(path string, key *model.APIKey) *httptest.ResponseRecorder {
		req := newLocalRequest("GET", path, nil)
		if key != nil {
			req = req.WithContext(auth.WithAPIKey(req.Context(), key))
		}
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		return w
	}

	var list struct {
		Models []geminiModel `json:"models"`
	}
	w := get("/v1beta/models", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("list decode: %v (%s)", err, w.Body.String())
	}
	byName := map[string]int{}
	for i, m := range list.Models {
		byName[m.Name] = i
	}

	// Google's listing includes image models, unlike our default chat surface.
	for _, want := range []string{"models/openai/gpt-4o", "models/openai/gpt-image-1", "models/openai/text-embedding-3-small"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("%s missing from /v1beta/models: %+v", want, list.Models)
		}
	}
	if len(list.Models) != 3 {
		t.Errorf("expected 3 models, got %d: %+v", len(list.Models), list.Models)
	}

	chat := list.Models[byName["models/openai/gpt-4o"]]
	if chat.DisplayName != "GPT-4o" || chat.InputTokenLimit != 128000 || chat.OutputTokenLimit != 16384 {
		t.Errorf("chat entry mismatched: %+v", chat)
	}
	if !reflect.DeepEqual(chat.SupportedGenerationMethods,
		[]string{"generateContent", "streamGenerateContent", "countTokens"}) {
		t.Errorf("chat methods = %v", chat.SupportedGenerationMethods)
	}

	img := list.Models[byName["models/openai/gpt-image-1"]]
	if !reflect.DeepEqual(img.SupportedGenerationMethods,
		[]string{"generateContent", "streamGenerateContent"}) {
		t.Errorf("image methods = %v", img.SupportedGenerationMethods)
	}

	emb := list.Models[byName["models/openai/text-embedding-3-small"]]
	if !reflect.DeepEqual(emb.SupportedGenerationMethods,
		[]string{"embedContent", "batchEmbedContents"}) {
		t.Errorf("embedding methods = %v", emb.SupportedGenerationMethods)
	}

	t.Run("retrieve accepts the listed resource name", func(t *testing.T) {
		for _, path := range []string{
			"/v1beta/models/models/openai/gpt-4o", // exactly what list returned
			"/v1beta/models/openai/gpt-4o",        // qualified without the resource prefix
			"/v1beta/models/gpt-4o",               // bare upstream id
		} {
			w := get(path, nil)
			if w.Code != http.StatusOK {
				t.Errorf("%s: expected 200, got %d: %s", path, w.Code, w.Body.String())
				continue
			}
			var got geminiModel
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("%s decode: %v", path, err)
			}
			if got.Name != "models/openai/gpt-4o" || got.DisplayName != "GPT-4o" {
				t.Errorf("%s resolved to %+v", path, got)
			}
		}
	})

	t.Run("scoped keys and unknown ids", func(t *testing.T) {
		scoped := &model.APIKey{ID: "scoped", AllowedModels: []string{"openai/gpt-4o"}}
		w := get("/v1beta/models", scoped)
		if w.Code != http.StatusOK {
			t.Fatalf("scoped list: %d %s", w.Code, w.Body.String())
		}
		if names := jsonNamesOfModels(w.Body.Bytes()); !reflect.DeepEqual(names, []string{"models/openai/gpt-4o"}) {
			t.Errorf("scoped list should expose only the allowed model, got %v", names)
		}
		if w := get("/v1beta/models/models/openai/gpt-image-1", scoped); w.Code != http.StatusNotFound {
			t.Errorf("scoped-out retrieve: expected 404, got %d: %s", w.Code, w.Body.String())
		}

		w = get("/v1beta/models/models/openai/nope", nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("unknown: expected 404, got %d: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Error struct {
				Code    int    `json:"code"`
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode error body: %v", err)
		}
		if resp.Error.Code != http.StatusNotFound || resp.Error.Status != "NOT_FOUND" {
			t.Errorf("unexpected google error shape: %+v", resp.Error)
		}
		if resp.Error.Message != `Model "openai/nope" not found.` {
			t.Errorf("unexpected message %q", resp.Error.Message)
		}
	})
}

func jsonNamesOfModels(raw []byte) []string {
	var resp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil
	}
	out := make([]string, 0, len(resp.Models))
	for _, m := range resp.Models {
		out = append(out, m.Name)
	}
	return out
}
