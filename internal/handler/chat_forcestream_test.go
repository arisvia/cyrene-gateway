package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

func TestForceStreamAggregation(t *testing.T) {
	// Upstream mock that only accepts streaming requests and returns SSE chunks
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["stream"] != true {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"only streaming supported"}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "data: {\"id\":\"chunk-1\",\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		fmt.Fprintf(w, "data: {\"id\":\"chunk-1\",\"choices\":[{\"delta\":{\"content\":\" World!\"}}]}\n\n")
		fmt.Fprintf(w, "data: {\"id\":\"chunk-1\",\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2,\"total_tokens\":7}}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()

	srv, database := setupTestServer(t)
	defer database.Close()

	// Register a mock provider with ForceStream: true
	orig, hasOrig := provider.Registry["test-forcestream"]
	provider.Registry["test-forcestream"] = provider.ProviderInfo{
		ID:          "test-forcestream",
		Name:        "Test ForceStream",
		BaseURL:     upstream.URL,
		APIType:     "openai",
		AuthType:    "api-key",
		ForceStream: true,
	}
	defer func() {
		if hasOrig {
			provider.Registry["test-forcestream"] = orig
		} else {
			delete(provider.Registry, "test-forcestream")
		}
	}()

	conn := &model.ProviderConnection{
		ID:       "test-fs-conn",
		Provider: "test-forcestream",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey: "sk-test",
		},
	}
	if err := database.CreateConnection(conn); err != nil {
		t.Fatalf("save conn: %v", err)
	}

	// Client sends NON-STREAMING request (stream: false)
	reqBody := `{"model":"test-forcestream/model-a","messages":[{"role":"user","content":"Hi"}],"stream":false}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(resp.Choices) == 0 {
		t.Fatalf("expected choices, got none")
	}
	if resp.Choices[0].Message.Content != "Hello World!" {
		t.Errorf("expected 'Hello World!', got %q", resp.Choices[0].Message.Content)
	}
	if resp.Usage.TotalTokens != 7 {
		t.Errorf("expected 7 tokens, got %d", resp.Usage.TotalTokens)
	}

	// Verify requestDetails table recorded this request
	time.Sleep(10 * time.Millisecond)
	rdRes, err := database.GetRequestDetails(db.RequestDetailFilter{
		Provider: "test-forcestream",
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("GetRequestDetails: %v", err)
	}
	if rdRes.Pagination.TotalItems != 1 {
		t.Errorf("expected 1 request detail, got %d", rdRes.Pagination.TotalItems)
	}
}
