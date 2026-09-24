package provider

// Qoder chat executor logic ported from 9router (executors/qoder.js +
// services/qoderModels.js). Handles request body construction, live model
// config fetching, and SSE envelope unwrapping.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/arisvia/cyrene-gateway/internal/translator"
)

// --- Model config cache (from services/qoderModels.js) ---

type qoderCatalogEntry struct {
	expiresAt  time.Time
	rawConfigs map[string]map[string]any
}

var (
	qoderCatalogMu    sync.Mutex
	qoderCatalogCache = make(map[string]*qoderCatalogEntry)
)

const qoderCatalogTTL = time.Hour

func qoderCacheKey(providerID, userID string) string {
	if providerID == "" {
		providerID = "qoder"
	}
	return "qoder:" + providerID + ":" + userID
}

// QoderModelConfig returns the server-published model_config for a model key,
// fetching the live catalog if needed. Returns nil when unavailable.
// Supports direct key match as well as display name / alias fuzzy matching.
func QoderModelConfig(creds QoderCosyCreds, modelKey string, client *http.Client) map[string]any {
	entry := qoderResolveCatalog(creds, client, false)
	if entry == nil {
		return nil
	}
	if config, ok := entry.rawConfigs[modelKey]; ok {
		out := make(map[string]any, len(config)+1)
		maps.Copy(out, config)
		out["key"] = modelKey
		return out
	}

	// Fuzzy match by model name / display name (e.g. Qwen3.7-Max -> qmodel_latest)
	lowerTarget := strings.ToLower(modelKey)
	for k, cfg := range entry.rawConfigs {
		if name, ok := cfg["name"].(string); ok && strings.EqualFold(name, lowerTarget) {
			out := make(map[string]any, len(cfg)+1)
			maps.Copy(out, cfg)
			out["key"] = k
			return out
		}
	}

	return nil
}

func qoderResolveCatalog(creds QoderCosyCreds, client *http.Client, force bool) *qoderCatalogEntry {
	key := qoderCacheKey(creds.ProviderID, creds.UserID)
	qoderCatalogMu.Lock()
	if !force {
		if entry, ok := qoderCatalogCache[key]; ok && time.Now().Before(entry.expiresAt) {
			qoderCatalogMu.Unlock()
			return entry
		}
	}
	qoderCatalogMu.Unlock()

	if client == nil {
		client = SafeHTTPClient(15*time.Second, false)
	}

	// Fetch model list with COSY signing (empty body).
	modelListURL := QoderModelListURL(creds.ProviderID, creds.AuthToken)
	headers, err := BuildQoderCosyHeaders(nil, modelListURL, creds)
	if err != nil {
	}

	req, err := http.NewRequest("GET", modelListURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var catalog struct {
		Chat []map[string]any `json:"chat"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil || len(catalog.Chat) == 0 {
		return nil
	}

	rawConfigs := make(map[string]map[string]any)
	for _, entry := range catalog.Chat {
		k, _ := entry["key"].(string)
		if k == "" {
			continue
		}
		rawConfigs[k] = entry
	}

	result := &qoderCatalogEntry{
		expiresAt:  time.Now().Add(qoderCatalogTTL),
		rawConfigs: rawConfigs,
	}

	qoderCatalogMu.Lock()
	qoderCatalogCache[key] = result
	qoderCatalogMu.Unlock()

	return result
}

// InvalidateQoderCatalog clears the cached catalog for a user.
func InvalidateQoderCatalog(userID string) {
	qoderCatalogMu.Lock()
	delete(qoderCatalogCache, qoderCacheKey("qoder", userID))
	delete(qoderCatalogCache, qoderCacheKey("qoder-cn", userID))
	qoderCatalogMu.Unlock()
}

// --- Request body construction (from executors/qoder.js) ---

// BuildQoderRequestBody maps an OpenAI-style request into the shape Qoder
// expects. Returns the encoded body bytes (latin1) and the model key.
func BuildQoderRequestBody(model string, body map[string]any, creds QoderCosyCreds, client *http.Client) (encodedBody []byte, qoderKey string, modelSource string, err error) {
	qoderKey = strings.TrimPrefix(model, "qoder/")

	// Fetch live model config
	modelConfig := QoderModelConfig(creds, qoderKey, client)
	if modelConfig == nil {
		// Force refresh once — cache may not be populated yet
		entry := qoderResolveCatalog(creds, client, true)
		if entry != nil {
			if cfg, ok := entry.rawConfigs[qoderKey]; ok {
				modelConfig = make(map[string]any, len(cfg)+1)
				maps.Copy(modelConfig, cfg)
				modelConfig["key"] = qoderKey
			}
		}
	}
	if modelConfig == nil {
		return nil, "", "", fmt.Errorf("qoder: model_config for %q not yet known (check upstream connectivity)", qoderKey)
	}

	messages, systemText := qoderNormalizeMessages(body)
	tools, _ := body["tools"].([]any)

	isReasoning, _ := modelConfig["is_reasoning"].(bool)
	maxOutputTokens := toInt(modelConfig["max_output_tokens"])

	maxTokens := 32768
	if maxOutputTokens > 0 {
		maxTokens = maxOutputTokens
	}
	if v := toInt(body["max_tokens"]); v > 0 && v < maxTokens {
		maxTokens = v
	}
	if v := toInt(body["max_completion_tokens"]); v > 0 && v < maxTokens {
		maxTokens = v
	}

	lastUser := qoderLastUserText(messages)
	sessionID := qoderStableHash("qoder-session", creds.UserID, qoderKey)
	recordID := qoderStableChatRecordID(qoderKey, messages, maxTokens)

	modelSource = "system"
	if s, ok := modelConfig["source"].(string); ok && s != "" {
		modelSource = s
	}
	payload := map[string]any{
		"request_id":       uuid.New().String(),
		"request_set_id":   recordID,
		"chat_record_id":   recordID,
		"session_id":       sessionID,
		"stream":           true,
		"chat_task":        "FREE_INPUT",
		"is_reply":         true,
		"is_retry":         false,
		"source":           1,
		"version":          "3",
		"session_type":     "qodercli",
		"agent_id":         "agent_common",
		"task_id":          "common",
		"code_language":    "",
		"chat_prompt":      "",
		"image_urls":       nil,
		"aliyun_user_type": "",
		"system":           systemText,
		"messages":         messages,
		"tools":            tools,
		"parameters":       map[string]any{"max_tokens": maxTokens},
		"chat_context": map[string]any{
			"chatPrompt": "",
			"imageUrls":  nil,
			"extra": map[string]any{
				"context":         []any{},
				"modelConfig":     map[string]any{"key": qoderKey, "is_reasoning": isReasoning},
				"originalContent": lastUser,
			},
			"features": []any{},
			"text":     lastUser,
		},
		"model_config": modelConfig,
		"business": map[string]any{
			"product":  "cli",
			"version":  "1.0.0",
			"type":     "agent",
			"stage":    "start",
			"id":       uuid.New().String(),
			"name":     qoderTruncate(lastUser, 30),
			"begin_at": time.Now().UnixMilli(),
		},
	}

	plainBody, err := json.Marshal(payload)
	if err != nil {
		return nil, "", "", fmt.Errorf("qoder: failed to marshal payload: %w", err)
	}

	encoded := QoderEncodeBody(plainBody)
	return []byte(encoded), qoderKey, modelSource, nil
}

// qoderNormalizeMessages hoists system messages out of the array (Qoder
// rejects system in messages) and flattens multipart content.
func qoderNormalizeMessages(body map[string]any) (messages []map[string]any, systemText string) {
	raw, _ := body["messages"].([]any)
	var systemParts []string

	for _, m := range raw {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		text := qoderExtractText(msg["content"])
		role, _ := msg["role"].(string)
		if role == "system" {
			if text != "" {
				systemParts = append(systemParts, text)
			}
			continue
		}
		cloned := make(map[string]any, len(msg))
		maps.Copy(cloned, msg)
		cloned["content"] = text
		messages = append(messages, cloned)
	}
	return messages, strings.Join(systemParts, "\n\n")
}

func qoderExtractText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case nil:
		return ""
	case []any:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprintf("%v", v)
	}
}

func qoderLastUserText(messages []map[string]any) string {
	for _, message := range slices.Backward(messages) {
		if message["role"] == "user" {
			if s, ok := message["content"].(string); ok {
				return s
			}
		}
	}
	return ""
}

func qoderStableHash(prefix string, parts ...string) string {
	h := sha256.New()
	h.Write([]byte(prefix))
	for _, p := range parts {
		h.Write([]byte{0})
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func qoderStableChatRecordID(model string, messages []map[string]any, maxTokens int) string {
	h := sha256.New()
	h.Write([]byte("qoder-record\x00"))
	h.Write([]byte(model))
	for _, m := range messages {
		if role, ok := m["role"].(string); ok {
			h.Write([]byte{0})
			h.Write([]byte(role))
		}
		if content, ok := m["content"].(string); ok && content != "" {
			h.Write([]byte{0})
			h.Write([]byte(content))
		}
	}
	h.Write([]byte(fmt.Sprintf("\x00mt=%d", maxTokens)))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func qoderTruncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case float32:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	}
	return 0
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f
		}
	}
	return 0
}

// --- SSE envelope unwrapping (from executors/qoder.js wrapQoderSSE) ---

// UnwrapQoderSSELine processes one upstream SSE line from Qoder's
// {statusCodeValue, body} envelope. Returns:
//   - data: the unwrapped OpenAI chunk JSON (empty if line should be skipped)
//   - done: true for a terminal frame; empty data means normal [DONE], while
//     non-empty data is a protocol error that MUST be handled before completion.
func UnwrapQoderSSELine(line, _ string) (data string, done bool) {
	payload, isDone, ok := translator.ParseSSEDataLineString(line)
	if !ok {
		return "", false
	}
	if isDone {
		return "", true
	}

	terminalError := func(status int, message string) (string, bool) {
		if len(message) > 200 {
			message = message[:200]
		}
		data, _ := json.Marshal(map[string]any{"error": map[string]any{
			"message": message, "type": "upstream_error", "code": status,
		}})
		return string(data), true
	}
	var envelope struct {
		Body            string `json:"body"`
		StatusCodeValue int    `json:"statusCodeValue"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return terminalError(http.StatusBadGateway, "invalid qoder SSE envelope")
	}

	statusVal := envelope.StatusCodeValue
	if statusVal == 0 {
		statusVal = http.StatusOK
	}
	inner := strings.TrimSpace(envelope.Body)
	if isQoderBillingBlock(inner) {
		return terminalError(http.StatusForbidden, inner)
	}
	if statusVal != http.StatusOK {
		if inner == "" {
			inner = fmt.Sprintf("upstream status %d", statusVal)
		}
		return terminalError(statusVal, inner)
	}
	if inner == "" {
		return "", false
	}
	if inner == "[DONE]" {
		return "", true
	}
	var chunk map[string]json.RawMessage
	if err := json.Unmarshal([]byte(inner), &chunk); err != nil || chunk == nil {
		return terminalError(http.StatusBadGateway, "invalid qoder SSE chunk")
	}

	// Strip embedded newlines so the SSE frame stays a single event.
	sanitized := strings.NewReplacer("\r\n", "", "\n", "", "\r", "").Replace(inner)
	return sanitized, len(chunk["error"]) > 0 && string(chunk["error"]) != "null"
}

func isQoderBillingBlock(inner string) bool {
	lower := strings.ToLower(inner)
	return strings.Contains(lower, "pricingurl") ||
		strings.Contains(lower, `"code":"110"`) ||
		strings.Contains(lower, `"code":"112"`) ||
		strings.Contains(lower, `"code":"10605"`) ||
		strings.Contains(lower, "insufficient") ||
		strings.Contains(lower, "arrears")
}
