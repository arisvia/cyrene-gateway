package handler

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
	"github.com/arisvia/cyrene-gateway/internal/translator"
	"github.com/arisvia/cyrene-gateway/internal/usage"
)

// handleAntigravityChat handles inference requests via Google Cloud Code Assist (Antigravity).
// It returns retryable=true when the account itself failed in a way another
// account could survive (quota exhausted, rate limited). In that case nothing has
// been written to w yet, so the caller may retry with the next connection. A false
// return means the response is already complete.
func (s *Server) handleAntigravityChat(
	w http.ResponseWriter,
	r *http.Request,
	req ChatCompletionRequest,
	rawBody []byte,
	modelInfo model.ModelInfo,
	conn *model.ProviderConnection,
	providerInfo provider.ProviderInfo,
) (retryable bool) {
	token := conn.Data.AccessToken
	if token == "" && conn.Data.APIKey != "" {
		token = conn.Data.APIKey
	}
	if token == "" {
		// No usable credential on this account: another one may have one.
		slog.Warn("Antigravity connection has no usable credential, hopping", slog.String("connection", conn.ID))
		return true
	}

	clientTimeout := 3 * time.Minute
	if req.Stream {
		clientTimeout = 0
	}
	client := s.getHTTPClient(clientTimeout)
	projectID := ""
	if conn.Data.ProviderSpecificData != nil {
		if pid, ok := conn.Data.ProviderSpecificData["projectId"].(string); ok && pid != "" {
			projectID = pid
		}
	}
	if projectID == "" {
		pid, err := s.EnsureAntigravityProject(r.Context(), conn, client)
		if err != nil {
			if r.Context().Err() != nil {
				return
			}
			slog.Warn("Antigravity project discovery failed", "error", err, "connection", conn.ID)
			if conn.Data.BaseURL == "" && providerInfo.BaseURL == "" {
				// Account-scoped failure (throttled/revoked entitlement on THIS
				// Google account): a sibling connection may carry a usable
				// project. Never surface a deterministic client-facing 400 for
				// it; let the pool walk proceed.
				return true
			}
		} else {
			projectID = pid
		}
	}

	// Transform OpenAI messages into Gemini contents format
	contents, systemInstruction := convertOpenAIToGeminiContents(req.Messages, modelInfo.Model)

	genConfig := map[string]any{}
	// Gemini 4 removes external sampling params (temperature/topP); forwarding them causes 400 INVALID_ARGUMENT (9router #4802 parity)
	if !isGemini4Model(modelInfo.Model) {
		if req.Temperature != nil {
			genConfig["temperature"] = *req.Temperature
		}
		if req.TopP != nil {
			genConfig["topP"] = *req.TopP
		}
	}
	if len(req.Stop) > 0 {
		genConfig["stopSequences"] = req.Stop
	}
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		genConfig["maxOutputTokens"] = *req.MaxTokens
	}
	sessionID := fmt.Sprintf("-%d", time.Now().UnixNano()%9000000000000000000)
	innerRequest := map[string]any{
		"contents":  contents,
		"sessionId": sessionID,
	}
	if len(systemInstruction) > 0 {
		innerRequest["systemInstruction"] = map[string]any{
			"parts": systemInstruction,
		}
	}
	// Forward tool definitions if present
	if len(req.Tools) > 0 {
		var rawTools []map[string]any
		if err := json.Unmarshal(req.Tools, &rawTools); err == nil && len(rawTools) > 0 {
			var declarations []any
			for _, t := range rawTools {
				var name, description string
				var params map[string]any

				if fn, ok := t["function"].(map[string]any); ok {
					name, _ = fn["name"].(string)
					description, _ = fn["description"].(string)
					params, _ = fn["parameters"].(map[string]any)
				}
				if name == "" {
					name, _ = t["name"].(string)
					description, _ = t["description"].(string)
					if p, ok := t["parameters"].(map[string]any); ok {
						params = p
					} else if s, ok := t["input_schema"].(map[string]any); ok {
						params = s
					}
				}
				if name != "" {
					if params != nil {
						params = translator.CleanJSONSchemaForGemini(params)
					}
					declarations = append(declarations, map[string]any{
						"name":        name,
						"description": description,
						"parameters":  params,
					})
				}
			}
			if len(declarations) > 0 {
				innerRequest["tools"] = []any{map[string]any{"functionDeclarations": declarations}}
			}
		}
	}

	availableModels := s.getAntigravityAvailableModels()
	targetModel, tier, shouldInjectThinking, isImage := resolveAntigravityModel(modelInfo.Model, req.ReasoningEffort, availableModels)

	if !isImage {
		innerRequest["model"] = modelInfo.Model
		s.applyLoopGuard(innerRequest, rawBody, "gemini", req.Tools != nil)
		s.applyTokenSaver(innerRequest, "gemini", modelInfo.Provider, r)
		delete(innerRequest, "model")
	}
	if isImage {
		genConfig["maxOutputTokens"] = 8192
		genConfig["imageConfig"] = map[string]any{"aspectRatio": "1:1"}
	} else if shouldInjectThinking && genConfig != nil && (tier == "high" || tier == "medium" || tier == "low") {
		genConfig["thinkingConfig"] = map[string]any{
			"thinkingLevel":   tier,
			"includeThoughts": true,
		}
		// Ensure maxOutputTokens strictly exceeds thinking budget to prevent 400 INVALID_ARGUMENT (9router#3981)
		minBudget := 4096
		if tier == "high" {
			minBudget = 16384
		} else if tier == "medium" {
			minBudget = 8192
		}
		if mt, ok := genConfig["maxOutputTokens"].(int); ok && mt <= minBudget {
			genConfig["maxOutputTokens"] = minBudget + 8192
		}
	}
	if len(genConfig) > 0 {
		innerRequest["generationConfig"] = genConfig
	}

	upstreamAction := "streamGenerateContent?alt=sse"
	if isImage {
		upstreamAction = "generateContent"
	}

	reqID := fmt.Sprintf("agent-%d-%s", time.Now().UnixMilli(), randomHex(4))
	envelope := map[string]any{
		"model":     targetModel,
		"request":   innerRequest,
		"userAgent": "antigravity",
		"requestId": reqID,
	}
	if projectID != "" {
		envelope["project"] = projectID
	}
	if isImage {
		envelope["requestType"] = "image_gen"
	}

	envelopeBytes, err := json.Marshal(envelope)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encode antigravity request"})
		return
	}

	baseURL := provider.AntigravityBaseURL
	if conn.Data.BaseURL != "" {
		baseURL = strings.TrimRight(conn.Data.BaseURL, "/")
	} else if providerInfo.BaseURL != "" {
		baseURL = strings.TrimRight(providerInfo.BaseURL, "/")
	}
	upstreamURL := fmt.Sprintf("%s/v1internal:%s", baseURL, upstreamAction)
	upReq, err := http.NewRequestWithContext(r.Context(), "POST", upstreamURL, bytes.NewReader(envelopeBytes))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upstream request"})
		return
	}

	upReq.Header.Set("Authorization", "Bearer "+token)
	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("Accept", "text/event-stream")
	upReq.Header.Set("User-Agent", provider.AntigravityUserAgent)

	endpoint := resolveRequestEndpoint(r, "/v1/chat/completions")
	start := time.Now()
	s.publishRequestEvent(r, RequestEvent{Type: "routing", Provider: conn.Provider, Model: modelInfo.Model, Endpoint: endpoint, Status: "routing"})
	resp, err := client.Do(upReq)
	if err != nil {
		if r.Context().Err() != nil {
			return false
		}
		// Transport failures are per-account as often as per-network; let the
		// caller try the next account before surfacing a 502.
		slog.Warn("Antigravity upstream connection failed", "error", err, "connection", conn.ID)
		return true
	}
	defer resp.Body.Close()

	// Phase 9: On-401 retry with token refresh for OAuth connections
	if resp.StatusCode == http.StatusUnauthorized && conn.Data.RefreshToken != "" {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		// Keep the original 401 body armed: any path that does not replace resp
		// (refresh failed, retry build/do failed) still records the real cause
		// instead of cooling the account on an empty error body.
		resp.Body = io.NopCloser(bytes.NewReader(errBody))
		slog.Warn("Antigravity upstream returned 401, attempting token refresh and retry",
			slog.String("provider", conn.Provider),
			slog.String("connection_id", conn.ID))
		refreshResult, refreshErr := provider.RefreshCredentials(conn.Provider, conn, s.getHTTPClient(30*time.Second))
		if refreshErr == nil {
			if err := s.persistRefreshedCredentials(conn, refreshResult); err != nil {
				slog.Error("Failed to persist refreshed Antigravity credentials", "error", err)
				return true
			}
			token = conn.Data.AccessToken
			retryReq, retryErr := http.NewRequestWithContext(r.Context(), "POST", upstreamURL, bytes.NewReader(envelopeBytes))
			if retryErr == nil {
				retryReq.Header.Set("Authorization", "Bearer "+token)
				retryReq.Header.Set("Content-Type", "application/json")
				retryReq.Header.Set("Accept", "text/event-stream")
				retryReq.Header.Set("User-Agent", provider.AntigravityUserAgent)
				if newResp, newErr := client.Do(retryReq); newErr == nil {
					resp = newResp
				}
			}
		}
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		provider.ApplyErrorState(conn, resp.StatusCode, string(respBody))
		// Quota-class exhaustion is POOL-scoped on antigravity (Claude & GPT /
		// Gemini / Gemini-4-Argon are separate windows): park only this pool's
		// models so the account keeps serving its other pools, instead of a
		// global 15-min RateLimitedUntil meltdown.
		if pool := antigravityQuotaPool(targetModel); pool != "" &&
			provider.CheckFallbackError(resp.StatusCode, string(respBody), conn.Data.BackoffLevel).Cooldown >= provider.CooldownQuota {
			conn.Data.RateLimitedUntil = ""
			if lockPatch := s.lockAntigravityPool(conn, pool, targetModel, modelInfo.Model); lockPatch != nil && s.DB != nil {
				_ = s.DB.UpdateConnectionProviderData(conn.ID, lockPatch)
			}
		}
		if s.DB != nil {
			_ = s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
		}
		latencyMs := time.Since(start).Milliseconds()
		if latencyMs < 0 {
			latencyMs = 0
		}
		statusStr := fmt.Sprintf("%d", resp.StatusCode)
		rdID := fmt.Sprintf("%d-%s", time.Now().UnixNano(), modelInfo.Model)
		rdData := map[string]any{
			"id":           rdID,
			"timestamp":    time.Now().UTC().Format(time.RFC3339),
			"provider":     conn.Provider,
			"model":        modelInfo.Model,
			"connectionId": conn.ID,
			"status":       statusStr,
			"latencyMs":    latencyMs,
			"endpoint":     endpoint,
			"error":        string(respBody),
			"input":        extractPromptSummary(req.Messages),
		}
		rdBytes, _ := json.Marshal(rdData)
		if s.DB != nil {
			_ = s.DB.SaveRequestDetail(&db.RequestDetail{
				ID:           rdID,
				Timestamp:    rdData["timestamp"].(string),
				Provider:     conn.Provider,
				Model:        modelInfo.Model,
				ConnectionID: conn.ID,
				Status:       statusStr,
				Data:         string(rdBytes),
			})
		}
		if s.Metrics != nil {
			s.Metrics.ObserveRequest(conn.Provider, modelInfo.Model, endpoint, resp.StatusCode, time.Since(start).Seconds(), nil)
		}
		s.publishRequestEvent(r, RequestEvent{Type: "request", Provider: conn.Provider, Model: modelInfo.Model, Endpoint: endpoint, Status: statusStr, LatencyMs: latencyMs})

		// The account itself is out of quota or throttled: another account in the
		// pool may still serve this request, so let the caller retry before the
		// client sees the failure.
		if provider.CheckFallbackError(resp.StatusCode, string(respBody), conn.Data.BackoffLevel).ShouldFallback {
			return true
		}
		writeJSON(w, resp.StatusCode, map[string]any{
			"error": fmt.Sprintf("antigravity upstream error (HTTP %d): %s", resp.StatusCode, string(respBody)),
		})
		return false
	}

	// Response headers arrived: the account served us. Clear sticky error state
	// (testStatus/lastError/backoff) like every other executor does; without it
	// a single malformed-history 400 pinned these connections as "error" until
	// an admin re-test, and backoffLevel only ever grew.
	provider.ResetAccountState(conn)
	if s.DB != nil {
		_ = s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
	}

	uc := &usageContext{
		StartedAt:    start,
		Provider:     conn.Provider,
		Model:        modelInfo.Model,
		ConnectionID: conn.ID,
		APIKey:       extractRequestAPIKey(r),
		Endpoint:     endpoint,
		Status:       http.StatusOK,
		Prompt:       extractPromptSummary(req.Messages),
	}

	if isImage || !req.Stream {
		if s.proxyAntigravityNonStreaming(w, resp, modelInfo.Model, uc, conn) {
			return true
		}
	} else {
		s.proxyAntigravityStreaming(w, r, resp, modelInfo.Model, uc, conn)
	}
	return false
}

// defaultThinkingAgSignature is the base64-encoded dummy thought signature ("skip_thought_signature_validator")
// officially recognized by Google Gemini and Vertex AI to bypass thought signature validation for prior turns.
const defaultThinkingAgSignature = "c2tpcF90aG91Z2h0X3NpZ25hdHVyZV92YWxpZGF0b3I="

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

// contentPartsForGemini flattens an OpenAI message's content into Gemini parts,
// carrying base64 image_url items as inlineData. Text-only flattening silently
// dropped vision input (the model answered about images it never received) and
// could leave an image-only turn with zero parts, which the turn normalizer
// deletes — producing an empty-contents 400 upstream.
func contentPartsForGemini(raw json.RawMessage) []map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		if str == "" {
			return nil
		}
		return []map[string]any{{"text": str}}
	}
	var items []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return []map[string]any{{"text": string(raw)}}
	}
	var out []map[string]any
	for _, it := range items {
		if it.ImageURL.URL != "" {
			if mt, data, ok := splitBase64DataURI(it.ImageURL.URL); ok {
				out = append(out, map[string]any{
					"inlineData": map[string]any{"mimeType": mt, "data": data},
				})
			}
			continue
		}
		if it.Type == "text" || it.Text != "" {
			out = append(out, map[string]any{"text": it.Text})
		}
	}
	return out
}

func splitBase64DataURI(u string) (string, string, bool) {
	if !strings.HasPrefix(u, "data:") {
		return "", "", false
	}
	comma := strings.IndexByte(u, ',')
	semi := strings.IndexByte(u, ';')
	if comma < 0 || semi < 5 || semi > comma {
		return "", "", false
	}
	return u[5:semi], u[comma+1:], true
}

func convertOpenAIToGeminiContents(messages []Message, targetModel ...string) ([]map[string]any, []map[string]any) {
	var contents []map[string]any
	var systemParts []map[string]any
	isGemini4 := len(targetModel) > 0 && isGemini4Model(targetModel[0])
	toolCallNames := make(map[string]string)
	// cloudcode-pa turns functionCall parts into Anthropic tool_use blocks where
	// id is REQUIRED (missing → 400 "messages.N.content.M.tool_use.id: Field
	// required"). OpenAI leaves tool_call_id optional, so calls without one get
	// a synthesized id here; the matching tool turn is paired back to it.
	callIDsBySeq := make(map[int]string)
	callSeq := 0
	for _, msg := range messages {
		if len(msg.ToolCalls) > 0 {
			var tcs []struct {
				ID       string `json:"id"`
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			}
			if err := json.Unmarshal(msg.ToolCalls, &tcs); err == nil {
				for _, tc := range tcs {
					id := tc.ID
					if id == "" {
						id = fmt.Sprintf("call_%s", randomHex(8))
						callIDsBySeq[callSeq] = id
					}
					if tc.Function.Name != "" {
						toolCallNames[id] = tc.Function.Name
					}
					callSeq++
				}
			}
		}
	}
	callSeq = 0 // reuse the counter for the emission pass below (same order)

	for _, msg := range messages {
		role := strings.ToLower(msg.Role)
		text := parseMessageContent(msg.Content)

		if role == "system" || role == "developer" {
			if text != "" {
				systemParts = append(systemParts, map[string]any{"text": text})
			}
			continue
		}
		if role == "tool" {
			fnName := toolCallNames[msg.ToolCallID]
			if fnName == "" {
				fnName = msg.ToolCallID
			}
			var resp any
			dec := json.NewDecoder(strings.NewReader(text))
			dec.UseNumber() // keep bare-number tool outputs exact (no float64 mangling)
			if err := dec.Decode(&resp); err != nil || resp == nil {
				resp = map[string]any{"result": text}
			}
			respMap, ok := resp.(map[string]any)
			if !ok {
				respMap = map[string]any{"result": resp}
			}
			// Echo the call id so the backend can bind the tool_result back
			// to its tool_use.
			frBody := map[string]any{
				"name":     fnName,
				"response": respMap,
			}
			if msg.ToolCallID != "" {
				frBody["id"] = msg.ToolCallID
			}
			contents = append(contents, map[string]any{
				"role": "user",
				"parts": []map[string]any{
					{"functionResponse": frBody},
				},
			})
			continue
		}
		if role == "assistant" {
			role = "model"
		}

		var parts []map[string]any
		parts = append(parts, contentPartsForGemini(msg.Content)...)

		// Backfill thoughtSignature for tool calling history on Gemini 3+
		if len(msg.ToolCalls) > 0 {
			var toolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
			}
			if err := json.Unmarshal(msg.ToolCalls, &toolCalls); err == nil && len(toolCalls) > 0 {
				for i, tc := range toolCalls {
					fcID := tc.ID
					if fcID == "" {
						fcID = callIDsBySeq[callSeq]
					}
					callSeq++
					var args map[string]any
					// Accept both string-encoded and object-shaped arguments so a
					// Mistral-style body doesn't fail the whole turn's unmarshal and
					// silently orphan its functionResponses.
					var asStr string
					if len(tc.Function.Arguments) > 0 {
						if err := json.Unmarshal(tc.Function.Arguments, &asStr); err == nil {
							_ = json.Unmarshal([]byte(asStr), &args)
						} else {
							_ = json.Unmarshal(tc.Function.Arguments, &args)
						}
					}
					if args == nil {
						args = map[string]any{}
					}
					fcPart := map[string]any{
						"functionCall": map[string]any{
							"id":   fcID,
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

	// Normalization: merge adjacent same-role turns and ensure functionResponse is placed last in user turns for Gemini 4
	contents = normalizeAntigravityContents(contents, isGemini4)

	return contents, systemParts
}

// antigravityFinishToOpenAI maps a lowercased cloudcode-pa finishReason onto
// the OpenAI enum that the downstream Claude adapter keys on ("length"→
// max_tokens, "content_filter"→refusal). Passing raw gemini tokens through
// made truncated output look like a clean end_turn/tool_use completion.
func antigravityFinishToOpenAI(fr string) string {
	switch fr {
	case "max_tokens":
		return "length"
	case "safety", "recitation", "prohibited_content", "blocklist":
		return "content_filter"
	default:
		return "stop"
	}
}

// antigravityQuotaPool maps a resolved upstream model onto the Google quota
// pool it draws from (verified against live retrieveUserQuotaSummary buckets):
// Claude & GPT-OSS share one pool, the Gemini family (text/search/image)
// shares another, and Gemini 4 Argon carries its own weekly pool.
func antigravityQuotaPool(targetModel string) string {
	lower := strings.ToLower(targetModel)
	switch {
	case strings.HasPrefix(lower, "claude-"), strings.HasPrefix(lower, "gpt-"):
		return "claude"
	case strings.HasPrefix(lower, "gemini-4"):
		return "gemini4"
	case strings.HasPrefix(lower, "gemini-"):
		return "gemini"
	}
	return ""
}

// lockAntigravityPool parks every same-pool model on this connection via the
// existing model-lock mechanism (the credential selector already skips locked
// models per request). One exhausted pool must not melt down the whole
// account: the sibling pools keep their headroom and keep serving.
// Returns the provider-data patch to persist (nil when nothing to lock).
func (s *Server) lockAntigravityPool(conn *model.ProviderConnection, pool, targetModel, requestedModel string) map[string]any {
	until := time.Now().Add(provider.CooldownQuota).UTC().Format(time.RFC3339)
	patch := map[string]any{}
	lock := func(name string) {
		if name != "" && antigravityQuotaPool(name) == pool {
			patch["modelLock_"+name] = until
		}
	}
	lock(targetModel)
	lock(requestedModel)
	for _, m := range s.getAntigravityAvailableModels() {
		lock(m.ID)
	}
	if len(patch) == 0 {
		return nil
	}
	if conn.Data.ProviderSpecificData == nil {
		conn.Data.ProviderSpecificData = make(map[string]any)
	}
	for k, v := range patch {
		conn.Data.ProviderSpecificData[k] = v
	}
	return patch
}

// isGemini4Model reports whether a model name refers to Google Gemini 4 (Argon architecture).
func isGemini4Model(model string) bool {
	m := strings.ToLower(model)
	if slash := strings.LastIndex(m, "/"); slash >= 0 {
		m = m[slash+1:]
	}
	return strings.HasPrefix(m, "gemini-4") || strings.Contains(m, "/gemini-4")
}

// normalizeAntigravityContents merges adjacent turns of the same role and places functionResponse last for Gemini 4
func normalizeAntigravityContents(contents []map[string]any, isGemini4 bool) []map[string]any {
	if len(contents) == 0 {
		return contents
	}
	var merged []map[string]any
	for _, c := range contents {
		role, _ := c["role"].(string)
		parts, _ := c["parts"].([]map[string]any)
		if len(parts) == 0 {
			continue
		}
		if len(merged) > 0 && merged[len(merged)-1]["role"] == role {
			lastParts, _ := merged[len(merged)-1]["parts"].([]map[string]any)
			lastParts = append(lastParts, parts...)
			if isGemini4 && role == "user" {
				lastParts = moveFunctionResponsesLast(lastParts)
			}
			merged[len(merged)-1]["parts"] = lastParts
			continue
		}
		if isGemini4 && role == "user" {
			parts = moveFunctionResponsesLast(parts)
		}
		merged = append(merged, map[string]any{
			"role":  role,
			"parts": parts,
		})
	}
	return merged
}

func moveFunctionResponsesLast(parts []map[string]any) []map[string]any {
	var nonFR []map[string]any
	var fr []map[string]any
	for _, p := range parts {
		if _, ok := p["functionResponse"]; ok {
			fr = append(fr, p)
		} else {
			nonFR = append(nonFR, p)
		}
	}
	if len(fr) == 0 {
		return parts
	}
	return append(nonFR, fr...)
}

type antigravityCandidatePart struct {
	Text         string `json:"text"`
	Thought      bool   `json:"thought"`
	FunctionCall *struct {
		Name string         `json:"name"`
		Args map[string]any `json:"args"`
	} `json:"functionCall,omitempty"`
}

type antigravityCandidateItem struct {
	Content struct {
		Parts []antigravityCandidatePart `json:"parts"`
	} `json:"content"`
	FinishReason string `json:"finishReason"`
}

type antigravityChunkPayload struct {
	Response struct {
		Candidates []antigravityCandidateItem `json:"candidates"`
	} `json:"response"`
	Candidates []antigravityCandidateItem `json:"candidates"`
	// cloudcode-pa sometimes places the terminal signal at the payload root
	// instead of inside a candidate (recorded upstream samples prove both).
	FinishReason string `json:"finishReason"`
	// In-band error frames arrive on an HTTP 200 SSE; ignoring them made a
	// failed turn look like a clean empty completion.
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (p *antigravityChunkPayload) getCandidates() []antigravityCandidateItem {
	if len(p.Response.Candidates) > 0 {
		return p.Response.Candidates
	}
	return p.Candidates
}

// Antigravity may wrap Gemini's response, or put usageMetadata at the root.
func extractAntigravityUsage(data []byte) usage.Usage {
	// Detect whether usageMetadata is actually present (not absent/null).
	rootHas := hasNonNullField(data, "usageMetadata")

	var envelopeHas bool
	var responseRaw json.RawMessage
	var env struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(data, &env); err == nil && len(env.Response) > 0 && string(env.Response) != "null" {
		responseRaw = env.Response
		envelopeHas = hasNonNullField(responseRaw, "usageMetadata")
	}

	var u usage.Usage
	if envelopeHas {
		u = usage.ExtractFromGemini(responseRaw)
	} else if rootHas {
		u = usage.ExtractFromGemini(data)
	}
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	if envelopeHas || rootHas {
		u.Known = true
	}
	return u
}

// hasNonNullField reports whether obj contains key with a non-null value.
func hasNonNullField(obj json.RawMessage, key string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(obj, &m); err != nil {
		return false
	}
	raw, ok := m[key]
	return ok && len(raw) > 0 && string(raw) != "null" && string(raw) != "{}"
}

func (s *Server) proxyAntigravityStreaming(w http.ResponseWriter, r *http.Request, resp *http.Response, model string, uc *usageContext, conn *model.ProviderConnection) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Cyrene-Served-Model", model)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	stallReader := provider.NewStallTimeoutReader(resp.Body, provider.DefaultStreamStallTimeout)
	defer stallReader.Close()
	scanner := bufio.NewScanner(stallReader)
	scanner.Buffer(make([]byte, 1024*1024), 5*1024*1024)

	chatCmpleID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixMilli())
	var lastUsage usage.Usage
	var outBuilder strings.Builder
	var reasoningBuilder strings.Builder
	// Monotonic tool-call slot across the whole stream: antigravity delivers each
	// functionCall as its own complete part, often in separate SSE events. Reusing
	// the per-event part offset would make clients (and the OpenAI→Claude stream
	// adapter) merge parallel calls into one with concatenated arguments.
	toolCallSeq := 0
	sawDone := false
	// A terminal finish_reason chunk actually written to the client; suppresses
	// the abort paths after a served end-of-turn (e.g. SAFETY refusal with zero parts).
	sawFinish := false
	ctx := r.Context()
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			slog.Info("Client disconnected during Antigravity stream", slog.String("model", model))
			finalOutput := outBuilder.String()
			if finalOutput == "" && reasoningBuilder.Len() > 0 {
				finalOutput = reasoningBuilder.String()
			}
			if uc != nil {
				uc.Status = 499
				uc.Response = finalOutput
			}
			s.recordUsage(uc, lastUsage)
			return
		default:
		}
		line := scanner.Bytes()
		if isSSEComment(line) {
			if err := forwardSSEComment(w, flusher, line); err != nil {
				s.markAntigravityClientGone(uc, &outBuilder, &reasoningBuilder)
				s.recordUsage(uc, lastUsage)
				return
			}
			continue
		}
		data, isDone, ok := translator.ParseSSEDataLineString(scanner.Text())
		if isDone {
			sawDone = true
			continue
		}
		if !ok {
			continue
		}

		var payload antigravityChunkPayload
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			continue
		}
		if u := extractAntigravityUsage([]byte(data)); u.Known || u.TotalTokens > 0 {
			// Never let a present-but-zeroed trailing frame wipe good usage.
			if u.TotalTokens > 0 || lastUsage.TotalTokens == 0 {
				lastUsage = u
			}
		}
		if payload.Error != nil && (payload.Error.Message != "" || payload.Error.Status != "" || payload.Error.Code >= 400) {
			errMsg := payload.Error.Message
			if errMsg == "" {
				errMsg = fmt.Sprintf("antigravity in-band error (code %d, status %s)", payload.Error.Code, payload.Error.Status)
			}
			if payload.Error.Status != "" && !strings.Contains(errMsg, payload.Error.Status) {
				// Carry the RPC enum into the classified text so the quota rule
				// (resource_exhausted → 15-min park) fires just like it does for
				// a real non-200 body; prose alone ("exhausted (e.g. check quota)")
				// would regress the account to a 30s transient park.
				errMsg += " (" + payload.Error.Status + ")"
			}
			s.abortAntigravityStream(w, flusher, uc, conn, model, &outBuilder, &reasoningBuilder, lastUsage, errMsg, antigravityInBandStatus(payload.Error.Code, payload.Error.Status))
			return
		}

		textChunk := ""
		reasoningChunk := ""
		finishReason := ""
		var toolCallsDelta []any
		for _, cand := range payload.getCandidates() {
			if cand.FinishReason != "" {
				finishReason = strings.ToLower(cand.FinishReason)
				sawFinish = true
			}
			for _, part := range cand.Content.Parts {
				if part.Thought {
					reasoningChunk += part.Text
				} else if part.FunctionCall != nil {
					tcIdx := toolCallSeq
					toolCallSeq++
					argsBytes, _ := json.Marshal(part.FunctionCall.Args)
					argsStr := string(argsBytes)
					if argsStr == "null" || argsStr == "" {
						// Zero-argument calls must stay JSON objects; "null"
						// becomes tool_use.input:null and breaks Anthropic clients.
						argsStr = "{}"
					}
					toolCallsDelta = append(toolCallsDelta, map[string]any{
						"index": tcIdx,
						"id":    fmt.Sprintf("call_%s_%d", randomHex(8), tcIdx),
						"type":  "function",
						"function": map[string]any{
							"name":      part.FunctionCall.Name,
							"arguments": argsStr,
						},
					})
				} else {
					textChunk += part.Text
				}
			}
		}

		if finishReason == "" && payload.FinishReason != "" {
			// Root-level terminal signal (recorded upstream shape).
			finishReason = strings.ToLower(payload.FinishReason)
		}

		if textChunk == "" && reasoningChunk == "" && len(toolCallsDelta) == 0 && finishReason == "" {
			continue
		}

		delta := map[string]any{}
		if textChunk != "" {
			delta["content"] = textChunk
			if outBuilder.Len() < 16384 {
				outBuilder.WriteString(textChunk)
			}
		}
		if reasoningChunk != "" {
			delta["reasoning_content"] = reasoningChunk
			if reasoningBuilder.Len() < 16384 {
				reasoningBuilder.WriteString(reasoningChunk)
			}
		}
		if len(toolCallsDelta) > 0 {
			delta["tool_calls"] = toolCallsDelta
		}

		var fr any = nil
		if finishReason != "" {
			fr = antigravityFinishToOpenAI(finishReason)
			if fr == "stop" && len(toolCallsDelta) > 0 {
				fr = "tool_calls"
			}
		} else if len(toolCallsDelta) > 0 {
			fr = "tool_calls"
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
		if _, writeErr := fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes)); writeErr != nil {
			s.markAntigravityClientGone(uc, &outBuilder, &reasoningBuilder)
			s.recordUsage(uc, lastUsage)
			return
		}
		if fr != nil {
			sawFinish = true
		}
	}

	if r.Context().Err() != nil {
		// Client hung up: book 499 and never cool the account for it.
		s.markAntigravityClientGone(uc, &outBuilder, &reasoningBuilder)
		s.recordUsage(uc, lastUsage)
		return
	}

	if scanErr := scanner.Err(); scanErr != nil && !sawDone && !sawFinish {
		// Stalled mid-turn: without a terminal frame the client only sees the
		// stream stop and cannot distinguish truncation from a clean end.
		s.abortAntigravityStream(w, flusher, uc, conn, model, &outBuilder, &reasoningBuilder, lastUsage,
			fmt.Sprintf("antigravity stream interrupted: %v", scanErr), http.StatusBadGateway)
		return
	}
	if !sawDone && !sawFinish && outBuilder.Len() == 0 && reasoningBuilder.Len() == 0 && toolCallSeq == 0 {
		// Upstream closed after headers without a single delta and without
		// [DONE]. A tool-only turn legitimately ends on EOF without
		// finishReason, so emptiness — not the missing terminal frame — is
		// the truncation signal here.
		s.abortAntigravityStream(w, flusher, uc, conn, model, &outBuilder, &reasoningBuilder, lastUsage,
			"antigravity stream produced no content before closing", http.StatusBadGateway)
		return
	}

	if lastUsage.Known || lastUsage.TotalTokens > 0 {
		chunk, _ := json.Marshal(map[string]any{
			"id": chatCmpleID, "object": "chat.completion.chunk", "created": time.Now().Unix(),
			"model": model, "choices": []any{}, "usage": lastUsage.OpenAI(),
		})
		if _, writeErr := fmt.Fprintf(w, "data: %s\n\n", chunk); writeErr != nil {
			s.markAntigravityClientGone(uc, &outBuilder, &reasoningBuilder)
			s.recordUsage(uc, lastUsage)
			return
		}
	}
	// Send final [DONE]
	if _, writeErr := fmt.Fprintf(w, "data: [DONE]\n\n"); writeErr != nil {
		s.markAntigravityClientGone(uc, &outBuilder, &reasoningBuilder)
		s.recordUsage(uc, lastUsage)
		return
	}
	flusher.Flush()

	finalOutput := outBuilder.String()
	if finalOutput == "" && reasoningBuilder.Len() > 0 {
		finalOutput = reasoningBuilder.String()
	}
	if uc != nil {
		uc.Response = finalOutput
	}
	s.recordUsage(uc, lastUsage)
}

// markAntigravityClientGone books a client-cancelled stream as 499 without
// touching account health (parity with chat_messages.go / media_walk.go).
func (s *Server) markAntigravityClientGone(uc *usageContext, out, reasoning *strings.Builder) {
	finalOutput := out.String()
	if finalOutput == "" && reasoning.Len() > 0 {
		finalOutput = reasoning.String()
	}
	if uc != nil {
		uc.Status = 499
		uc.Response = finalOutput
	}
}

// abortAntigravityStream terminates an upstream-failed stream with an in-band
// error chunk (the OpenAI→Claude adapters render {"error":{...}} chunks as a
// proper Anthropic event:error) instead of a misleading [DONE], books the
// failure status, and records account error state.
func (s *Server) abortAntigravityStream(w http.ResponseWriter, flusher http.Flusher, uc *usageContext, conn *model.ProviderConnection, poolModel string, out, reasoning *strings.Builder, lastUsage usage.Usage, message string, status int) {
	slog.Warn("Antigravity stream aborted", slog.String("message", message), slog.Int("status", status))
	payload, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"type":    anthropicErrorTypeForStatus(status),
			"message": message,
			"status":  status,
		},
	})
	fmt.Fprintf(w, "data: %s\n\n", payload)
	flusher.Flush()

	if uc != nil {
		finalOutput := out.String()
		if finalOutput == "" && reasoning.Len() > 0 {
			finalOutput = reasoning.String()
		}
		uc.Status = status
		uc.Response = finalOutput
	}
	if conn != nil {
		provider.ApplyErrorState(conn, status, message)
		if pool := antigravityQuotaPool(poolModel); pool != "" &&
			provider.CheckFallbackError(status, message, conn.Data.BackoffLevel).Cooldown >= provider.CooldownQuota {
			conn.Data.RateLimitedUntil = ""
			if lockPatch := s.lockAntigravityPool(conn, pool, poolModel, poolModel); lockPatch != nil && s.DB != nil {
				_ = s.DB.UpdateConnectionProviderData(conn.ID, lockPatch)
			}
		}
		if s.DB != nil {
			_ = s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
		}
	}
	s.recordUsage(uc, lastUsage)
}

// antigravityInBandStatus derives an HTTP-equivalent status from an in-band
// error frame. cloudcode-pa often carries only the RPC enum, so a
// Code==0 RESOURCE_EXHAUSTED frame must still surface as 429 for both the
// client-visible error type and the fallback/quota rules.
func antigravityInBandStatus(code int, status string) int {
	if code >= 400 && code <= 599 {
		return code
	}
	switch strings.ToUpper(status) {
	case "RESOURCE_EXHAUSTED", "RATE_LIMIT_EXCEEDED":
		return http.StatusTooManyRequests
	case "UNAVAILABLE":
		return http.StatusServiceUnavailable
	case "INTERNAL", "DATA_LOSS", "UNKNOWN":
		return http.StatusInternalServerError
	case "PERMISSION_DENIED", "FAILED_PRECONDITION":
		return http.StatusForbidden
	case "UNAUTHENTICATED":
		return http.StatusUnauthorized
	case "INVALID_ARGUMENT":
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

func (s *Server) proxyAntigravityNonStreaming(w http.ResponseWriter, resp *http.Response, model string, uc *usageContext, conn *model.ProviderConnection) (retryable bool) {
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to read antigravity response: " + err.Error()})
		return false
	}

	fullText := ""
	reasoningText := ""
	var toolCalls []any
	var lastUsage usage.Usage
	upstreamFinish := ""
	upstreamErr := ""
	upstreamErrCode := 0
	upstreamErrStatus := ""

	extractParts := func(data []byte) {
		var payload antigravityChunkPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return
		}
		if u := extractAntigravityUsage(data); u.Known || u.TotalTokens > 0 {
			if u.TotalTokens > 0 || lastUsage.TotalTokens == 0 {
				lastUsage = u
			}
		}
		if payload.Error != nil && (payload.Error.Message != "" || payload.Error.Status != "" || payload.Error.Code >= 400) {
			upstreamErr = payload.Error.Message
			if upstreamErr == "" {
				upstreamErr = fmt.Sprintf("antigravity in-band error (code %d, status %s)", payload.Error.Code, payload.Error.Status)
			}
			if payload.Error.Status != "" && !strings.Contains(upstreamErr, payload.Error.Status) {
				upstreamErr += " (" + payload.Error.Status + ")"
			}
			upstreamErrCode = payload.Error.Code
			upstreamErrStatus = payload.Error.Status
		}
		for _, cand := range payload.getCandidates() {
			if cand.FinishReason != "" {
				upstreamFinish = strings.ToLower(cand.FinishReason)
			}
			if upstreamFinish == "" && payload.FinishReason != "" {
				upstreamFinish = strings.ToLower(payload.FinishReason)
			}
			for i, part := range cand.Content.Parts {
				if part.Thought {
					reasoningText += part.Text
				} else if part.FunctionCall != nil {
					argsBytes, _ := json.Marshal(part.FunctionCall.Args)
					argsStr := string(argsBytes)
					if argsStr == "null" || argsStr == "" {
						argsStr = "{}"
					}
					toolCalls = append(toolCalls, map[string]any{
						"id":   fmt.Sprintf("call_%s_%d", randomHex(8), i),
						"type": "function",
						"function": map[string]any{
							"name":      part.FunctionCall.Name,
							"arguments": argsStr,
						},
					})
				} else {
					fullText += part.Text
				}
			}
		}
	}

	trimmed := bytes.TrimSpace(respBytes)
	if bytes.HasPrefix(trimmed, []byte("{")) {
		extractParts(trimmed)
	} else {
		scanner := bufio.NewScanner(bytes.NewReader(respBytes))
		scanner.Buffer(make([]byte, 1024*1024), 5*1024*1024)
		for scanner.Scan() {
			data, isDone, ok := translator.ParseSSEDataLineString(scanner.Text())
			if !ok || isDone {
				continue
			}
			extractParts([]byte(data))
		}
	}

	// Any in-band error frame fails the non-streaming turn: nothing has been
	// written to w yet, so quota/throttle classes hand off to the sibling
	// connection, and the rest surface honestly instead of fabricating a
	// clean "stop" completion out of a truncated partial buffer (F21).
	if upstreamErr != "" {
		status := antigravityInBandStatus(upstreamErrCode, upstreamErrStatus)
		if conn != nil {
			provider.ApplyErrorState(conn, status, upstreamErr)
			if pool := antigravityQuotaPool(model); pool != "" &&
				provider.CheckFallbackError(status, upstreamErr, conn.Data.BackoffLevel).Cooldown >= provider.CooldownQuota {
				conn.Data.RateLimitedUntil = ""
				if lockPatch := s.lockAntigravityPool(conn, pool, model, model); lockPatch != nil && s.DB != nil {
					_ = s.DB.UpdateConnectionProviderData(conn.ID, lockPatch)
				}
			}
			if s.DB != nil {
				_ = s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
			}
			if provider.CheckFallbackError(status, upstreamErr, conn.Data.BackoffLevel).ShouldFallback {
				return true
			}
		}
		if uc != nil {
			uc.Status = status
		}
		s.recordUsage(uc, lastUsage)
		writeJSON(w, status, map[string]any{
			"error": map[string]any{
				"type":    anthropicErrorTypeForStatus(status),
				"message": upstreamErr,
			},
		})
		return false
	}

	finishReason := "stop"
	if upstreamFinish != "" {
		finishReason = antigravityFinishToOpenAI(upstreamFinish)
	}
	if finishReason == "stop" && len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	respObj := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixMilli()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": func() map[string]any {
					m := map[string]any{
						"role":    "assistant",
						"content": fullText,
					}
					if reasoningText != "" {
						m["reasoning_content"] = reasoningText
					}
					if len(toolCalls) > 0 {
						m["tool_calls"] = toolCalls
					}
					return m
				}(),
				"finish_reason": finishReason,
			},
		},
	}

	if lastUsage.Known || lastUsage.TotalTokens > 0 {
		respObj["usage"] = lastUsage.OpenAI()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cyrene-Served-Model", model)
	json.NewEncoder(w).Encode(respObj)
	finalResp := fullText
	if finalResp == "" && reasoningText != "" {
		finalResp = reasoningText
	}
	if uc != nil {
		uc.Response = finalResp
	}
	s.recordUsage(uc, lastUsage)
	return false
}
func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func (s *Server) getAntigravityAvailableModels() []model.ModelMetadata {
	if s.DB == nil {
		return nil
	}
	if raw, err := s.DB.KVGet("providerModelCache", "antigravity"); err == nil && raw != "" {
		var cached model.CachedModels
		if err := json.Unmarshal([]byte(raw), &cached); err == nil && len(cached.Models) > 0 {
			return cached.Models
		}
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
		if tier != "" && !strings.HasSuffix(cleanModel, "-"+tier) {
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
