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
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const qoderModelListURL = "https://api3.qoder.sh/algo/api/v2/model/list"

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

func qoderCacheKey(userID string) string {
	return "qoder:" + userID
}

// QoderModelConfig returns the server-published model_config for a model key,
// fetching the live catalog if needed. Returns nil when unavailable.
func QoderModelConfig(creds QoderCosyCreds, modelKey string, client *http.Client) map[string]any {
	entry := qoderResolveCatalog(creds, client, false)
	if entry == nil {
		return nil
	}
	config, ok := entry.rawConfigs[modelKey]
	if !ok {
		return nil
	}
	// Defensive copy with key set
	out := make(map[string]any, len(config)+1)
	for k, v := range config {
		out[k] = v
	}
	out["key"] = modelKey
	return out
}

func qoderResolveCatalog(creds QoderCosyCreds, client *http.Client, force bool) *qoderCatalogEntry {
	key := qoderCacheKey(creds.UserID)

	qoderCatalogMu.Lock()
	if !force {
		if entry, ok := qoderCatalogCache[key]; ok && time.Now().Before(entry.expiresAt) {
			qoderCatalogMu.Unlock()
			return entry
		}
	}
	qoderCatalogMu.Unlock()

	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	// Fetch model list with COSY signing (empty body)
	headers, err := BuildQoderCosyHeaders(nil, qoderModelListURL, creds)
	if err != nil {
		return nil
	}

	req, err := http.NewRequest("GET", qoderModelListURL, nil)
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
	delete(qoderCatalogCache, qoderCacheKey(userID))
	qoderCatalogMu.Unlock()
}

// --- Request body construction (from executors/qoder.js) ---

// BuildQoderRequestBody maps an OpenAI-style request into the shape Qoder
// expects. Returns the encoded body bytes (latin1) and the model key.
func BuildQoderRequestBody(model string, body map[string]any, creds QoderCosyCreds, client *http.Client) (encodedBody []byte, qoderKey string, err error) {
	qoderKey = strings.TrimPrefix(model, "qoder/")

	// Fetch live model config
	modelConfig := QoderModelConfig(creds, qoderKey, client)
	if modelConfig == nil {
		// Force refresh once — cache may not be populated yet
		entry := qoderResolveCatalog(creds, client, true)
		if entry != nil {
			if cfg, ok := entry.rawConfigs[qoderKey]; ok {
				modelConfig = make(map[string]any, len(cfg)+1)
				for k, v := range cfg {
					modelConfig[k] = v
				}
				modelConfig["key"] = qoderKey
			}
		}
	}
	if modelConfig == nil {
		return nil, "", fmt.Errorf("qoder: model_config for %q not yet known (check upstream connectivity)", qoderKey)
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

	modelSource := "system"
	if s, ok := modelConfig["source"].(string); ok && s != "" {
		modelSource = s
	}
	_ = modelSource

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
		return nil, "", fmt.Errorf("qoder: failed to marshal payload: %w", err)
	}

	encoded := QoderEncodeBody(plainBody)
	return []byte(encoded), qoderKey, nil
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
		for k, v := range msg {
			cloned[k] = v
		}
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
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i]["role"] == "user" {
			if s, ok := messages[i]["content"].(string); ok {
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
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

// --- SSE envelope unwrapping (from executors/qoder.js wrapQoderSSE) ---

// UnwrapQoderSSELine processes one upstream SSE line from Qoder's
// {statusCodeValue, body} envelope. Returns:
//   - data: the unwrapped OpenAI chunk JSON (empty if line should be skipped)
//   - done: true when the stream has ended ([DONE])
func UnwrapQoderSSELine(line, model string) (data string, done bool) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if trimmed == "" || !strings.HasPrefix(trimmed, "data:") {
		return "", false
	}

	payload := strings.TrimSpace(trimmed[len("data:"):])
	if payload == "[DONE]" {
		return "", true
	}

	var envelope struct {
		StatusCodeValue int    `json:"statusCodeValue"`
		Body            string `json:"body"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return "", false
	}

	statusVal := envelope.StatusCodeValue
	if statusVal == 0 {
		statusVal = 200
	}
	inner := envelope.Body

	if statusVal != 200 {
		msg := inner
		if msg == "" {
			msg = fmt.Sprintf("upstream status %d", statusVal)
		}
		if len(msg) > 200 {
			msg = msg[:200]
		}
		errChunk, _ := json.Marshal(map[string]any{
			"id":      fmt.Sprintf("qoder-error-%d", time.Now().UnixMilli()),
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{{
				"index":         0,
				"delta":         map[string]any{"content": fmt.Sprintf("\n[qoder error %d: %s]", statusVal, msg)},
				"finish_reason": "stop",
			}},
		})
		return string(errChunk), true
	}

	if inner == "" {
		return "", false
	}
	if inner == "[DONE]" {
		return "", true
	}

	// Strip embedded newlines so the SSE frame stays a single event
	sanitized := strings.NewReplacer("\r\n", "", "\n", "", "\r", "").Replace(inner)
	return sanitized, false
}
