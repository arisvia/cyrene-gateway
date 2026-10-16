package provider

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/loopguard"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/rtk"
	"github.com/arisvia/cyrene-gateway/internal/translator"
	"github.com/google/uuid"
)

// ExecutionCandidate represents a resolved model & connection attempt candidate.
type ExecutionCandidate struct {
	Connection   *model.ProviderConnection
	ModelInfo    model.ModelInfo
	ModelStr     string
	ProviderInfo ProviderInfo
}

// ExecutionRequest holds the normalized request payload and metadata.
type ExecutionRequest struct {
	Model      string
	RawBody    []byte
	Messages   []map[string]any
	Stream     bool
	HasTools   bool
	SaveTokens bool
	// LoopGuardEnabled mirrors db.Settings.LoopGuardEnabled; the executor has no
	// settings access, so the caller resolves the switch once per request.
	LoopGuardEnabled bool
	AutoPromptCache  bool
}

// ApplyLoopGuard composes the repetition hint and the termination contract into a
// prepared upstream body of any supported shape. Detection lives in loopguard; the
// rewriting is shared with the token-saver injections through rtk.
func ApplyLoopGuard(body map[string]any, rawBody []byte, format string, hasTools bool) {
	var history struct {
		Messages []loopguard.Message `json:"messages"`
	}
	if err := json.Unmarshal(rawBody, &history); err != nil {
		return
	}
	if result := loopguard.DetectLoop(history.Messages); result.Detected {
		rtk.InjectSystemPrompt(body, format, result.Hint)
	}
	if hasTools {
		rtk.InjectSystemPrompt(body, format, loopguard.TerminationPrompt)
	}
}

// ExecutionResult is the outcome of executing against an upstream provider.
type ExecutionResult struct {
	Response       *http.Response
	Connection     *model.ProviderConnection
	ModelInfo      model.ModelInfo
	TargetFormat   translator.Format
	ErrorBody      []byte
	StatusCode     int
	ShouldFallback bool
}

// PrepareUpstreamRequest translates payload, injects loopguard/termination/tokensaver, builds URL and applies auth.
func PrepareUpstreamRequest(
	ctx context.Context,
	req ExecutionRequest,
	cand ExecutionCandidate,
	applyTokenSaverFn func(map[string]any, string),
) (*http.Request, translator.Format, error) {
	conn := cand.Connection
	providerInfo := cand.ProviderInfo
	modelInfo := cand.ModelInfo

	baseURL, effectiveAPIType := providerInfo.EffectiveBaseURL(conn.AuthType, conn.Data.APIKey != "")
	if conn.Data.BaseURL != "" {
		baseURL = conn.Data.BaseURL
		effectiveAPIType = providerInfo.APIType
	}
	if baseURL == "" {
		return nil, "", fmt.Errorf("no base URL configured for provider: %s", modelInfo.Provider)
	}

	targetFormat := translator.FormatOpenAI
	switch effectiveAPIType {
	case "anthropic":
		targetFormat = translator.FormatAnthropic
	case "gemini":
		targetFormat = translator.FormatGemini
	case "responses":
		targetFormat = translator.FormatResponses
	}

	transport := ResolveTransport(providerInfo, baseURL, effectiveAPIType, conn)

	var bodyMap map[string]any
	if err := json.Unmarshal(req.RawBody, &bodyMap); err != nil {
		return nil, "", fmt.Errorf("invalid request body: %w", err)
	}
	if bodyMap == nil {
		return nil, "", fmt.Errorf("request body must be an object")
	}
	upstreamStream := req.Stream || providerInfo.ForceStream
	bodyMap["model"] = modelInfo.Model
	bodyMap["stream"] = upstreamStream
	if targetFormat == translator.FormatOpenAI && upstreamStream && !req.Stream {
		options, _ := bodyMap["stream_options"].(map[string]any)
		if options == nil {
			options = make(map[string]any)
		}
		options["include_usage"] = true
		bodyMap["stream_options"] = options
	}

	if modelInfo.Provider == "codex" {
		if tools, ok := bodyMap["tools"].([]any); ok {
			for _, tool := range tools {
				if tm, ok := tool.(map[string]any); ok {
					if fn, ok := tm["function"].(map[string]any); ok {
						if params, ok := fn["parameters"].(map[string]any); ok {
							translator.StripCodexUnsupportedPatterns(params)
						}
					}
				}
			}
		}
	}
	if strings.HasPrefix(modelInfo.Provider, "codebuddy") {
		sanitizeCodebuddyRequest(modelInfo.Provider, bodyMap)
	}
	if modelInfo.Provider == "github" {
		sanitizeGithubRequest(modelInfo.Model, bodyMap)
	}
	ClampMaxTokens(modelInfo.Provider, modelInfo.Model, bodyMap)
	// The sanitizers above may replace the system message, so the guard text is
	// composed after them and reaches the upstream in the surviving shape.
	if req.LoopGuardEnabled {
		ApplyLoopGuard(bodyMap, req.RawBody, "openai", req.HasTools)
	}
	if applyTokenSaverFn != nil {
		applyTokenSaverFn(bodyMap, "openai")
	}
	translated, err := translator.TranslateRequest(targetFormat, modelInfo.Model, bodyMap, upstreamStream)
	if err != nil {
		return nil, "", fmt.Errorf("translation failed: %w", err)
	}
	if targetFormat == translator.FormatAnthropic && req.AutoPromptCache {
		translator.AutoInjectClaudeCacheControl(translated, true)
	}
	if modelInfo.Provider == "codex" {
		translated["store"] = false
		if _, ok := translated["instructions"]; !ok {
			translated["instructions"] = ""
		}
	}
	bodyBytes, err := json.Marshal(translated)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal request: %w", err)
	}

	targetURL := BuildTransportURL(transport, modelInfo.Model, upstreamStream)
	upstreamReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create upstream request: %w", err)
	}
	for k, v := range transport.Headers {
		upstreamReq.Header.Set(k, v)
	}
	ApplyAuth(upstreamReq, transport, ResolveCredentials(conn, modelInfo.Provider, modelInfo.Model))
	upstreamReq.Header.Set("Content-Type", "application/json")
	if upstreamStream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	}
	if strings.HasPrefix(modelInfo.Provider, "codebuddy") {
		applyCodebuddyConversationHeaders(upstreamReq, bodyMap)
	}
	return upstreamReq, targetFormat, nil
}

// ExecuteAttempt executes a single candidate upstream attempt.
func ExecuteAttempt(
	ctx context.Context,
	client *http.Client,
	req ExecutionRequest,
	cand ExecutionCandidate,
	applyTokenSaverFn func(map[string]any, string),
) ExecutionResult {
	upstreamReq, targetFormat, err := PrepareUpstreamRequest(ctx, req, cand, applyTokenSaverFn)
	if err != nil {
		return ExecutionResult{
			StatusCode:     http.StatusBadRequest,
			ErrorBody:      []byte(err.Error()),
			ShouldFallback: false,
			ModelInfo:      cand.ModelInfo,
			Connection:     cand.Connection,
			TargetFormat:   targetFormat,
		}
	}

	resp, err := client.Do(upstreamReq)
	if err != nil {
		if ctx.Err() != nil {
			return ExecutionResult{
				StatusCode:     499,
				ErrorBody:      []byte("client canceled request"),
				ShouldFallback: false,
				ModelInfo:      cand.ModelInfo,
				Connection:     cand.Connection,
				TargetFormat:   targetFormat,
			}
		}
		slog.Warn("Upstream request network failure", slog.String("provider", cand.ModelInfo.Provider), "error", err)
		return ExecutionResult{
			StatusCode:     http.StatusBadGateway,
			ErrorBody:      []byte(err.Error()),
			ShouldFallback: true,
			ModelInfo:      cand.ModelInfo,
			Connection:     cand.Connection,
			TargetFormat:   targetFormat,
		}
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return ExecutionResult{
			Response:     resp,
			TargetFormat: targetFormat,
			ModelInfo:    cand.ModelInfo,
			Connection:   cand.Connection,
			StatusCode:   resp.StatusCode,
		}
	}

	// Read upstream error body (capped to 1MB to prevent OOM)
	errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()

	fbResult := CheckFallbackError(resp.StatusCode, string(errBody), cand.Connection.Data.BackoffLevel)

	return ExecutionResult{
		StatusCode:     resp.StatusCode,
		ErrorBody:      errBody,
		ShouldFallback: fbResult.ShouldFallback,
		ModelInfo:      cand.ModelInfo,
		Connection:     cand.Connection,
		TargetFormat:   targetFormat,
	}
}

var agentSystemPromptRegex = regexp.MustCompile(`(?i)you are claude code|claude.?code.+official.+cli|anthropic.+official.+cli|anxthxropic.+official.+cli|you are (?:cursor|windsurf|cline|aider|continue|copilot|cody)|you are an? (?:ai )?(?:coding |code )?agent|cc_entrypoint\s*=\s*(?:cli|vscode|jetbrains|gui)|claude.?code.+issues|give feedback.+claude.?code|you are .{0,30}(?:powerful )?ai agent|orchestration capabilities|OhMyOpenCode|<agent-identity>|<Role>|<Behavior_Instructions>`)

const neutralCodebuddyPrompt = "You are a helpful AI assistant that helps with software engineering tasks."

func sanitizeCodebuddyRequest(providerID string, bodyMap map[string]any) {
	bodyMap["stream"] = true

	if eff, ok := bodyMap["reasoning_effort"].(string); ok {
		effLower := strings.ToLower(eff)
		if effLower == "none" || effLower == "off" {
			delete(bodyMap, "reasoning_effort")
		} else if effLower != "" {
			bodyMap["reasoning_summary"] = "auto"
		}
	}

	rawMsgs, ok := bodyMap["messages"].([]any)
	if !ok || len(rawMsgs) == 0 {
		bodyMap["messages"] = []any{
			map[string]any{"role": "system", "content": "You are CodeBuddy Code."},
		}
		return
	}

	if providerID == "codebuddy" {
		var filtered []any
		filtered = append(filtered, map[string]any{"role": "system", "content": "You are CodeBuddy Code."})
		for _, m := range rawMsgs {
			msgMap, ok := m.(map[string]any)
			if !ok {
				continue
			}
			role, _ := msgMap["role"].(string)
			if role == "system" || role == "developer" {
				continue
			}
			if role == "user" {
				if contentStr, ok := msgMap["content"].(string); ok {
					newMsg := make(map[string]any, len(msgMap))
					for k, v := range msgMap {
						newMsg[k] = v
					}
					newMsg["content"] = []any{
						map[string]any{"type": "text", "text": contentStr},
					}
					filtered = append(filtered, newMsg)
					continue
				}
			}
			filtered = append(filtered, msgMap)
		}
		bodyMap["messages"] = filtered
	} else if providerID == "codebuddy-cn" {
		// Explicit: an unlisted codebuddy-* sibling must not silently inherit the
		// China shape. TestRegistryCoversEveryCodebuddyShape guards that.
		hasSystem := false
		for _, m := range rawMsgs {
			if msgMap, ok := m.(map[string]any); ok {
				if role, _ := msgMap["role"].(string); role == "system" {
					hasSystem = true
					break
				}
			}
		}
		messages := rawMsgs
		if !hasSystem {
			messages = append([]any{map[string]any{"role": "system", "content": "You are CodeBuddy Code."}}, rawMsgs...)
		}
		var sanitized []any
		for _, m := range messages {
			msgMap, ok := m.(map[string]any)
			if !ok {
				sanitized = append(sanitized, m)
				continue
			}
			role, _ := msgMap["role"].(string)
			if role == "system" {
				contentStr, _ := msgMap["content"].(string)
				if len(contentStr) > 2000 || agentSystemPromptRegex.MatchString(contentStr) {
					newMsg := make(map[string]any, len(msgMap))
					for k, v := range msgMap {
						newMsg[k] = v
					}
					newMsg["content"] = neutralCodebuddyPrompt
					sanitized = append(sanitized, newMsg)
					continue
				}
			}
			sanitized = append(sanitized, msgMap)
		}
		bodyMap["messages"] = sanitized
	}
}

func sanitizeGithubRequest(model string, bodyMap map[string]any) {
	m := strings.ToLower(model)
	if strings.Contains(m, "gpt-5") || strings.Contains(m, "o1") || strings.Contains(m, "o3") || strings.Contains(m, "o4") {
		if maxTokens, ok := bodyMap["max_tokens"]; ok {
			bodyMap["max_completion_tokens"] = maxTokens
			delete(bodyMap, "max_tokens")
		}
	}
	if eff, ok := bodyMap["reasoning_effort"].(string); ok && strings.ToLower(eff) == "none" {
		delete(bodyMap, "reasoning_effort")
	}
}

func stableCodebuddyUUID(key string) string {
	h := sha1.Sum([]byte(key))
	h[6] = (h[6] & 0x0f) | 0x50
	h[8] = (h[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

func applyCodebuddyConversationHeaders(req *http.Request, bodyMap map[string]any) {
	var conversationKey string
	for _, key := range []string{"session_id", "prompt_cache_key", "conversation_id"} {
		if val, ok := bodyMap[key].(string); ok && val != "" {
			conversationKey = val
			break
		}
	}
	if conversationKey == "" {
		if msgs, ok := bodyMap["messages"].([]any); ok {
			for _, m := range msgs {
				if msgMap, ok := m.(map[string]any); ok {
					if role, _ := msgMap["role"].(string); role == "user" {
						switch c := msgMap["content"].(type) {
						case string:
							if c != "" {
								conversationKey = "cbai:" + c
							}
						case []any:
							var parts []string
							for _, b := range c {
								if bMap, ok := b.(map[string]any); ok {
									if text, ok := bMap["text"].(string); ok && text != "" {
										parts = append(parts, text)
									}
								}
							}
							if len(parts) > 0 {
								conversationKey = "cbai:" + strings.Join(parts, "\n")
							}
						}
						if conversationKey != "" {
							break
						}
					}
				}
			}
		}
	}

	convID := uuid.New().String()
	if conversationKey != "" {
		convID = stableCodebuddyUUID(conversationKey)
	}
	req.Header.Set("X-Conversation-ID", convID)
	req.Header.Set("X-Conversation-Request-ID", uuid.New().String())
	req.Header.Set("X-Conversation-Message-ID", uuid.New().String())
}
