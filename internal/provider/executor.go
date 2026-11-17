package provider

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
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
		if effectiveAPIType == "" {
			effectiveAPIType = providerInfo.APIType
		}
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

	if modelInfo.Provider == "openai" && conn != nil && conn.AuthType == "oauth" {
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
	if modelInfo.Provider == "openai" || strings.HasPrefix(modelInfo.Provider, "custom-openai") {
		sanitizeOpenAIRequest(modelInfo.Model, bodyMap)
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
	if modelInfo.Provider == "openai" && conn != nil && conn.AuthType == "oauth" {
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

const (
	neutralCodebuddyPrompt = "You are a helpful AI assistant that helps with software engineering tasks."

	// codebuddyIdentityPrompt satisfies the Intl gateway's request shape (it
	// answers 11101 to a body with no leading system message) and is used only
	// when the caller sent no system/developer text at all. It is not a mask for
	// the caller's own prompt.
	codebuddyIdentityPrompt = "You are CodeBuddy Code."

	// codebuddyCNMaxSystemLen is the CN-only length catch-all: the marker regex
	// enumerates known agents and cannot know the next one, so an over-long system
	// prompt is treated as some agent's instruction file. Upstream applies no such
	// gate to the Intl endpoint, so a long project prompt survives there.
	codebuddyCNMaxSystemLen = 2000
)

// neutralizeCodebuddySystem rewrites one system message's content when it reads
// as a third-party coding-agent identity. Tencent's WAF rejects those outright —
// 400 code 11128 "Illegal API invocation from an unapproved channel", or the
// sensitive-content refusal — and the trigger is the literal text, established by
// bisecting it upstream of this repo, not by our own guess. Content arrives either
// as a string or as typed text blocks depending on the inbound client shape, so
// the match runs on flattened text while the replacement keeps the original shape.
// maxLen <= 0 disables the length catch-all.
func neutralizeCodebuddySystem(content any, maxLen int) (any, bool) {
	switch content.(type) {
	case string, []any:
	default:
		return content, false
	}
	text := messageContentText(content)
	if text == "" {
		return content, false
	}
	if !(maxLen > 0 && len(text) > maxLen) && !agentSystemPromptRegex.MatchString(text) {
		return content, false
	}
	return reshapeCodebuddySystem(content, neutralCodebuddyPrompt), true
}

func reshapeCodebuddySystem(content any, text string) any {
	if _, ok := content.(string); ok {
		return text
	}
	return []any{map[string]any{"type": "text", "text": text}}
}

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
			map[string]any{"role": "system", "content": codebuddyIdentityPrompt},
		}
		return
	}

	switch providerID {
	case "codebuddy":
		bodyMap["messages"] = codebuddyIntlMessages(rawMsgs)
	case "codebuddy-cn":
		bodyMap["messages"] = codebuddyCNMessages(rawMsgs)
	default:
		// An unlisted codebuddy-* sibling keeps the plain OpenAI shape: neither the
		// Intl leading-system rewrite nor the CN length gate is guessed for it. It
		// still gets marker neutralization, because that is the half that costs an
		// account. TestRegistryCoversEveryCodebuddyShape pins all three paths.
		bodyMap["messages"] = neutralizeCodebuddySystems(rawMsgs, 0)
	}
}

// codebuddyIntlMessages folds the caller's own system/developer text into the
// single leading system message the Intl gateway demands, neutralizing agent
// identity markers inside it. The previous behaviour discarded that text
// unconditionally — no upstream basis, and it silently dropped the per-key
// systemPrompt an admin configures on an API key.
func codebuddyIntlMessages(rawMsgs []any) []any {
	var prompts []string
	out := make([]any, 0, len(rawMsgs)+1)
	for _, m := range rawMsgs {
		msgMap, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)
		if role == "system" || role == "developer" {
			text := messageContentText(msgMap["content"])
			if text == "" {
				continue
			}
			if agentSystemPromptRegex.MatchString(text) {
				text = neutralCodebuddyPrompt
			}
			prompts = append(prompts, text)
			continue
		}
		if role == "user" {
			if contentStr, ok := msgMap["content"].(string); ok {
				// User content travels as typed blocks; a bare string is rejected.
				clone := make(map[string]any, len(msgMap))
				maps.Copy(clone, msgMap)
				clone["content"] = []any{map[string]any{"type": "text", "text": contentStr}}
				out = append(out, clone)
				continue
			}
		}
		out = append(out, msgMap)
	}
	system := strings.Join(prompts, "\n\n")
	if system == "" {
		system = codebuddyIdentityPrompt
	}
	return append([]any{map[string]any{"role": "system", "content": system}}, out...)
}

// codebuddyCNMessages leaves the message array in place — the CN endpoint takes
// the plain OpenAI shape — only guarantees a leading system message, then
// neutralizes agent-shaped system text with the CN length catch-all enabled.
func codebuddyCNMessages(rawMsgs []any) []any {
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
		messages = append([]any{map[string]any{"role": "system", "content": codebuddyIdentityPrompt}}, rawMsgs...)
	}
	return neutralizeCodebuddySystems(messages, codebuddyCNMaxSystemLen)
}

// neutralizeCodebuddySystems rewrites agent-shaped system messages and leaves
// everything else, including user and assistant text, untouched.
func neutralizeCodebuddySystems(rawMsgs []any, maxLen int) []any {
	out := make([]any, 0, len(rawMsgs))
	for _, m := range rawMsgs {
		msgMap, ok := m.(map[string]any)
		if !ok {
			out = append(out, m)
			continue
		}
		if role, _ := msgMap["role"].(string); role != "system" {
			out = append(out, msgMap)
			continue
		}
		content, replaced := neutralizeCodebuddySystem(msgMap["content"], maxLen)
		if !replaced {
			out = append(out, msgMap)
			continue
		}
		clone := make(map[string]any, len(msgMap))
		maps.Copy(clone, msgMap)
		clone["content"] = content
		out = append(out, clone)
	}
	return out
}

func sanitizeGithubRequest(model string, bodyMap map[string]any) {
	m := strings.ToLower(model)
	if strings.Contains(m, "gpt-5") || strings.Contains(m, "gpt-6") || strings.Contains(m, "o1") || strings.Contains(m, "o3") || strings.Contains(m, "o4") {
		if maxTokens, ok := bodyMap["max_tokens"]; ok {
			if _, hasComp := bodyMap["max_completion_tokens"]; !hasComp {
				bodyMap["max_completion_tokens"] = maxTokens
			}
			delete(bodyMap, "max_tokens")
		}
	}
	if eff, ok := bodyMap["reasoning_effort"].(string); ok && strings.ToLower(eff) == "none" {
		delete(bodyMap, "reasoning_effort")
	}
}

// sanitizeOpenAIRequest normalizes parameters for modern OpenAI models (9router#7c42d8f8 parity).
// Current OpenAI models (gpt-5.x, gpt-6.x, o-series) reject max_tokens with HTTP 400.
func sanitizeOpenAIRequest(model string, bodyMap map[string]any) {
	m := strings.ToLower(model)
	if strings.Contains(m, "gpt-5") || strings.Contains(m, "gpt-6") || strings.Contains(m, "o1") || strings.Contains(m, "o3") || strings.Contains(m, "o4") {
		if maxTokens, ok := bodyMap["max_tokens"]; ok {
			if _, hasComp := bodyMap["max_completion_tokens"]; !hasComp {
				bodyMap["max_completion_tokens"] = maxTokens
			}
			delete(bodyMap, "max_tokens")
		}
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
	// Canonical conversation identity. Upstream tagging additionally accepts the
	// first user message as a last resort (routing affinity deliberately does
	// not, so similar openings of unrelated chats never share an account).
	conversationKey := SessionKeyFromBodyMap(bodyMap)
	if conversationKey == "" {
		if first := FirstUserText(bodyMap); first != "" {
			conversationKey = "cbai:" + first
		}
	}

	if req.Header.Get("X-Conversation-ID") == "" {
		convID := uuid.New().String()
		if conversationKey != "" {
			convID = stableCodebuddyUUID(conversationKey)
		}
		req.Header.Set("X-Conversation-ID", convID)
	}
	if req.Header.Get("X-Conversation-Request-ID") == "" {
		req.Header.Set("X-Conversation-Request-ID", uuid.New().String())
	}
	if req.Header.Get("X-Conversation-Message-ID") == "" {
		req.Header.Set("X-Conversation-Message-ID", uuid.New().String())
	}
}
