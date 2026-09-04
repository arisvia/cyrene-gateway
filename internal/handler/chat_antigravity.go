package handler

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
	"github.com/arisvia/cyrene-gateway/internal/usage"
)

// handleAntigravityChat handles inference requests via Google Cloud Code Assist (Antigravity).
func (s *Server) handleAntigravityChat(
	w http.ResponseWriter,
	r *http.Request,
	req ChatCompletionRequest,
	rawBody []byte,
	modelInfo model.ModelInfo,
	conn *model.ProviderConnection,
	providerInfo provider.ProviderInfo,
) {
	token := conn.Data.AccessToken
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "antigravity token missing"})
		return
	}

	projectID := ""
	if conn.Data.ProviderSpecificData != nil {
		if pid, ok := conn.Data.ProviderSpecificData["projectId"].(string); ok {
			projectID = pid
		}
	}

	client := s.getHTTPClient(3 * time.Minute)

	// Lazy project discovery if not cached
	if projectID == "" {
		discovered, err := DiscoverAntigravityProject(r.Context(), client, token)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("antigravity project not configured and discovery failed: %v", err),
			})
			return
		}
		projectID = discovered
		if conn.Data.ProviderSpecificData == nil {
			conn.Data.ProviderSpecificData = make(map[string]any)
		}
		conn.Data.ProviderSpecificData["projectId"] = projectID
		s.DB.UpdateConnection(conn)
	}

	// Transform OpenAI messages into Gemini contents format
	contents, systemInstruction := convertOpenAIToGeminiContents(req.Messages)

	genConfig := map[string]any{}
	if req.Temperature != nil {
		genConfig["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		genConfig["maxOutputTokens"] = *req.MaxTokens
	}

	innerRequest := map[string]any{
		"contents": contents,
	}
	if len(systemInstruction) > 0 {
		innerRequest["systemInstruction"] = map[string]any{
			"parts": systemInstruction,
		}
	}
	if len(genConfig) > 0 {
		innerRequest["generationConfig"] = genConfig
	}

	// Model mapping: strip prefixes
	targetModel := modelInfo.Model
	targetModel = strings.TrimPrefix(targetModel, "antigravity/")
	targetModel = strings.TrimPrefix(targetModel, "ag/")

	reqID := fmt.Sprintf("agent-%d-%s", time.Now().UnixMilli(), randomHex(4))
	envelope := map[string]any{
		"project":     projectID,
		"model":       targetModel,
		"request":     innerRequest,
		"requestType": "agent",
		"userAgent":   "antigravity",
		"requestId":   reqID,
	}

	envelopeBytes, err := json.Marshal(envelope)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encode antigravity request"})
		return
	}

	upstreamURL := fmt.Sprintf("%s/v1internal:streamGenerateContent?alt=sse", antigravityBaseURL)
	upReq, err := http.NewRequestWithContext(r.Context(), "POST", upstreamURL, bytes.NewReader(envelopeBytes))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upstream request"})
		return
	}

	upReq.Header.Set("Authorization", "Bearer "+token)
	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("Accept", "text/event-stream")
	upReq.Header.Set("User-Agent", antigravityUserAgent)
	upReq.Header.Set("X-Goog-Api-Client", antigravityXGoogClient)
	upReq.Header.Set("Client-Metadata", antigravityMetadata)

	resp, err := client.Do(upReq)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "antigravity upstream connection failed: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		writeJSON(w, resp.StatusCode, map[string]any{
			"error": fmt.Sprintf("antigravity upstream error (HTTP %d): %s", resp.StatusCode, string(respBody)),
		})
		return
	}

	uc := &usageContext{
		StartedAt:    time.Now(),
		Provider:     conn.Provider,
		Model:        modelInfo.Model,
		ConnectionID: conn.ID,
		Status:       http.StatusOK,
	}

	if req.Stream {
		s.proxyAntigravityStreaming(w, r, resp, modelInfo.Model, uc)
	} else {
		s.proxyAntigravityNonStreaming(w, resp, modelInfo.Model, uc)
	}
}
func convertOpenAIToGeminiContents(messages []Message) ([]map[string]any, []map[string]any) {
	var contents []map[string]any
	var systemParts []map[string]any

	for _, msg := range messages {
		role := strings.ToLower(msg.Role)
		if role == "system" {
			systemParts = append(systemParts, map[string]any{"text": msg.Content})
			continue
		}
		if role == "assistant" {
			role = "model"
		}
		contents = append(contents, map[string]any{
			"role": role,
			"parts": []map[string]any{
				{"text": msg.Content},
			},
		})
	}

	return contents, systemParts
}

func (s *Server) proxyAntigravityStreaming(w http.ResponseWriter, r *http.Request, resp *http.Response, model string, uc *usageContext) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Cyrene-Served-Model", model)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 5*1024*1024)

	chatSeq := 0
	chatCmpleID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixMilli())

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var payload struct {
			Response struct {
				Candidates []struct {
					Content struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					} `json:"content"`
				} `json:"candidates"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}

		textChunk := ""
		for _, cand := range payload.Response.Candidates {
			for _, part := range cand.Content.Parts {
				textChunk += part.Text
			}
		}

		if textChunk == "" {
			continue
		}

		chunk := map[string]any{
			"id":      chatCmpleID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{
				{
					"index": 0,
					"delta": map[string]any{
						"content": textChunk,
					},
					"finish_reason": nil,
				},
			},
		}

		chunkBytes, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
		flusher.Flush()
		chatSeq++
	}

	// Send final [DONE]
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	s.recordUsage(uc, usage.Usage{TotalTokens: chatSeq * 4})
}

func (s *Server) proxyAntigravityNonStreaming(w http.ResponseWriter, resp *http.Response, model string, uc *usageContext) {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 5*1024*1024)

	fullText := ""
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var payload struct {
			Response struct {
				Candidates []struct {
					Content struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					} `json:"content"`
				} `json:"candidates"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}

		for _, cand := range payload.Response.Candidates {
			for _, part := range cand.Content.Parts {
				fullText += part.Text
			}
		}
	}

	respObj := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixMilli()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": fullText,
				},
				"finish_reason": "stop",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cyrene-Served-Model", model)
	json.NewEncoder(w).Encode(respObj)

	s.recordUsage(uc, usage.Usage{TotalTokens: len(fullText) / 4})
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
