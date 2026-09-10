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
	availableModels := s.getAntigravityAvailableModels()
	targetModel, tier, shouldInjectThinking, isImage := resolveAntigravityModel(modelInfo.Model, req.ReasoningEffort, availableModels)

	if isImage {
		genConfig["maxOutputTokens"] = 8192
		genConfig["imageConfig"] = map[string]any{"aspectRatio": "1:1"}
	} else if shouldInjectThinking && genConfig != nil && (tier == "high" || tier == "medium" || tier == "low") {
		genConfig["thinkingConfig"] = map[string]any{
			"thinkingLevel":   tier,
			"includeThoughts": true,
		}
	}
	if len(genConfig) > 0 {
		innerRequest["generationConfig"] = genConfig
	}

	reqType := "agent"
	upstreamAction := "streamGenerateContent?alt=sse"
	if isImage {
		reqType = "image_gen"
		upstreamAction = "generateContent"
	}

	reqID := fmt.Sprintf("agent-%d-%s", time.Now().UnixMilli(), randomHex(4))
	envelope := map[string]any{
		"project":     projectID,
		"model":       targetModel,
		"request":     innerRequest,
		"requestType": reqType,
		"userAgent":   "antigravity",
		"requestId":   reqID,
	}

	envelopeBytes, err := json.Marshal(envelope)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encode antigravity request"})
		return
	}

	upstreamURL := fmt.Sprintf("%s/v1internal:%s", provider.AntigravityBaseURL, upstreamAction)
	upReq, err := http.NewRequestWithContext(r.Context(), "POST", upstreamURL, bytes.NewReader(envelopeBytes))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upstream request"})
		return
	}

	upReq.Header.Set("Authorization", "Bearer "+token)
	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("Accept", "text/event-stream")
	upReq.Header.Set("User-Agent", provider.AntigravityUserAgent)
	upReq.Header.Set("X-Goog-Api-Client", provider.AntigravityXGoogClient)
	upReq.Header.Set("Client-Metadata", provider.AntigravityMetadata)

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

	if isImage || !req.Stream {
		s.proxyAntigravityNonStreaming(w, resp, modelInfo.Model, uc)
	} else {
		s.proxyAntigravityStreaming(w, r, resp, modelInfo.Model, uc)
	}
}

const defaultThinkingAgSignature = "context_engineering_thought_signature"

func parseMessageContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return str
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "text" || p.Text != "" {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return string(raw)
}

func convertOpenAIToGeminiContents(messages []Message) ([]map[string]any, []map[string]any) {
	var contents []map[string]any
	var systemParts []map[string]any

	for _, msg := range messages {
		role := strings.ToLower(msg.Role)
		text := parseMessageContent(msg.Content)

		if role == "system" {
			systemParts = append(systemParts, map[string]any{"text": text})
			continue
		}
		if role == "assistant" {
			role = "model"
		} else if role == "tool" {
			role = "user"
		}

		parts := []map[string]any{
			{"text": text},
		}

		// Backfill thoughtSignature for tool calling history on Gemini 3+
		if len(msg.ToolCalls) > 0 {
			var toolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			}
			if err := json.Unmarshal(msg.ToolCalls, &toolCalls); err == nil && len(toolCalls) > 0 {
				for i, tc := range toolCalls {
					var args map[string]any
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
					if args == nil {
						args = map[string]any{}
					}
					fcPart := map[string]any{
						"functionCall": map[string]any{
							"name": tc.Function.Name,
							"args": args,
						},
					}
					if i == 0 {
						fcPart["thoughtSignature"] = defaultThinkingAgSignature
					}
					parts = append(parts, fcPart)
				}
			}
		}

		contents = append(contents, map[string]any{
			"role":  role,
			"parts": parts,
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
							Text    string `json:"text"`
							Thought bool   `json:"thought"`
						} `json:"parts"`
					} `json:"content"`
					FinishReason string `json:"finishReason"`
				} `json:"candidates"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}

		textChunk := ""
		reasoningChunk := ""
		finishReason := ""
		for _, cand := range payload.Response.Candidates {
			if cand.FinishReason != "" {
				finishReason = strings.ToLower(cand.FinishReason)
			}
			for _, part := range cand.Content.Parts {
				if part.Thought {
					reasoningChunk += part.Text
				} else {
					textChunk += part.Text
				}
			}
		}

		if textChunk == "" && reasoningChunk == "" && finishReason == "" {
			continue
		}

		delta := map[string]any{}
		if textChunk != "" {
			delta["content"] = textChunk
		}
		if reasoningChunk != "" {
			delta["reasoning_content"] = reasoningChunk
		}

		var fr any = nil
		if finishReason != "" {
			if finishReason == "stop" || finishReason == "max_tokens" {
				fr = finishReason
			} else {
				fr = "stop"
			}
		}

		chunk := map[string]any{
			"id":      chatCmpleID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{
				{
					"index":         0,
					"delta":         delta,
					"finish_reason": fr,
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
							Text    string `json:"text"`
							Thought bool   `json:"thought"`
						} `json:"parts"`
					} `json:"content"`
					FinishReason string `json:"finishReason"`
				} `json:"candidates"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}

		for _, cand := range payload.Response.Candidates {
			for _, part := range cand.Content.Parts {
				if !part.Thought {
					fullText += part.Text
				}
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

func (s *Server) getAntigravityAvailableModels() []model.ModelMetadata {
	if raw, err := s.DB.KVGet("providerModelCache", "antigravity"); err == nil && raw != "" {
		var cached model.CachedModels
		if err := json.Unmarshal([]byte(raw), &cached); err == nil && len(cached.Models) > 0 {
			return cached.Models
		}
	}
	if pInfo, ok := provider.GetProvider("antigravity"); ok && len(pInfo.Models) > 0 {
		return populateStaticModels(pInfo)
	}
	return nil
}

// resolveAntigravityModel dynamically resolves the target upstream model ID and tier from available models.
func resolveAntigravityModel(
	rawModel string,
	reasoningEffort string,
	availableModels []model.ModelMetadata,
) (targetModel string, tier string, shouldInjectThinking bool, isImage bool) {
	cleanModel := rawModel
	cleanModel = strings.TrimPrefix(cleanModel, "antigravity/")
	cleanModel = strings.TrimPrefix(cleanModel, "ag/")
	cleanModel = strings.TrimSpace(cleanModel)

	if strings.Contains(strings.ToLower(cleanModel), "image") {
		return cleanModel, "", false, true
	}

	lowerEffort := strings.ToLower(strings.TrimSpace(reasoningEffort))
	switch lowerEffort {
	case "high", "medium", "low", "extra-low":
		tier = lowerEffort
	}

	for _, t := range []string{"high", "medium", "low", "extra-low"} {
		if strings.HasSuffix(cleanModel, "-"+t) {
			tier = t
			break
		}
	}
	if tier == "" {
		tier = "medium"
	}
	availMap := make(map[string]model.ModelMetadata, len(availableModels))
	for _, m := range availableModels {
		availMap[m.ID] = m
	}

	// 1. Exact match
	if _, ok := availMap[cleanModel]; ok {
		targetModel = cleanModel
	}

	// 2. Effort suffix probing if not resolved yet
	if targetModel == "" {
		effectiveTier := tier
		if effectiveTier == "" {
			effectiveTier = "medium"
		}

		if cleanModel == "gemini-3.5-flash" {
			if effectiveTier == "high" {
				targetModel = "gemini-3.5-flash-high"
			} else if effectiveTier == "low" {
				targetModel = "gemini-3.5-flash-extra-low"
			} else {
				targetModel = "gemini-3.5-flash-low"
			}
		} else if cleanModel == "gemini-3.1-pro" {
			if effectiveTier == "low" {
				targetModel = "gemini-3.1-pro-low"
			} else {
				targetModel = "gemini-3.1-pro-high"
			}
		} else {
			probe := cleanModel + "-" + effectiveTier
			if _, ok := availMap[probe]; ok {
				targetModel = probe
			} else {
				for _, alt := range []string{"medium", "high", "low"} {
					probeAlt := cleanModel + "-" + alt
					if _, ok := availMap[probeAlt]; ok {
						targetModel = probeAlt
						if tier == "" {
							tier = alt
						}
						break
					}
				}
			}
		}
	}

	// 3. Prefix matching in available models
	if targetModel == "" {
		for id := range availMap {
			if strings.HasPrefix(id, cleanModel+"-") {
				targetModel = id
				break
			}
		}
	}

	// 4. Default / Passthrough fallback
	if targetModel == "" {
		if tier != "" && !strings.HasSuffix(cleanModel, "-"+tier) && strings.HasPrefix(cleanModel, "gemini-") {
			targetModel = cleanModel + "-" + tier
		} else {
			targetModel = cleanModel
		}
	}

	if tier == "" {
		for _, t := range []string{"high", "medium", "low", "extra-low"} {
			if strings.HasSuffix(targetModel, "-"+t) {
				tier = t
				break
			}
		}
	}
	if tier == "" {
		tier = "medium"
	}

	// Determine if thinkingConfig should be injected:
	// Only Gemini family models with thinking capability should receive thinkingConfig.
	// Claude, GPT-OSS, and non-thinking models must NEVER receive thinkingConfig.
	isGemini := strings.HasPrefix(targetModel, "gemini-")
	if meta, ok := availMap[targetModel]; ok {
		if meta.Family != "" && meta.Family != "gemini" {
			isGemini = false
		}
		hasReasoning := false
		for _, cap := range meta.Capabilities {
			if cap == "reasoning" {
				hasReasoning = true
				break
			}
		}
		shouldInjectThinking = isGemini && hasReasoning && (tier == "high" || tier == "medium" || tier == "low")
	} else {
		// Non-cached fallback heuristic: only gemini- models get thinkingConfig
		shouldInjectThinking = isGemini && (tier == "high" || tier == "medium" || tier == "low")
	}

	return targetModel, tier, shouldInjectThinking, false
}

// resolveAntigravityModelTier maps requested model and reasoning_effort to upstream tiered model (compat).
func resolveAntigravityModelTier(rawModel, reasoningEffort string) (targetModel string, tier string) {
	targetModel, tier, _, _ = resolveAntigravityModel(rawModel, reasoningEffort, nil)
	return targetModel, tier
}
