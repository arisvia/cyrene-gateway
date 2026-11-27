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
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

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
	if targetFormat == translator.FormatOpenAI && isStrictOpenAISchemaProvider(modelInfo.Provider) {
		sanitizeStrictOpenAIThinking(bodyMap)
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
	// Both steps above append to the instruction slot the sanitizer just
	// measured, so CodeBuddy CN's length catch-all is re-asserted here or the
	// gate is only ever applied to the pre-injection text.
	enforceCodebuddyCNLength(modelInfo.Provider, bodyMap)
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
	if modelInfo.Provider == "grok-cli" {
		applyGrokConversationHeaders(upstreamReq, bodyMap, conn)
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

// agentSystemPromptRegex enumerates the third-party coding-agent identities Tencent's
// WAF rejects with 400 code 11128. The alternatives are deliberately NOT ours to
// narrow: this pattern mirrors upstream 9router's AGENT_PATTERN
// (open-sse/executors/codebuddy-intl.js) byte-for-byte, and that project tunes it
// against live rejections we cannot see. Dropping a broad alternative such as
// `orchestration capabilities` or `<Role>` would forward text upstream still
// refuses and cost accounts.
//
// The only deviation is whitespace tolerance: upstream writes single spaces as
// literals, so "You are\nClaude Code" and "You are  Claude Code" slip past it —
// and `messageContentText` joins typed blocks with "\n", which manufactures
// exactly that form from a two-block identity header. Every `\s` here is one
// space widened to whitespace; nothing else was added or removed. The `.+` spans
// stay line-bound on purpose (no (?s)): a greedy cross-line span would delete
// whole paragraphs of the caller's prompt.
var agentSystemPromptRegex = regexp.MustCompile(`(?i)you are\s+claude\s+code|claude.?code.+official.+cli|anthropic.+official.+cli|anxthxropic.+official.+cli|you are\s+(?:cursor|windsurf|cline|aider|continue|copilot|cody)|you are\s+an?\s+(?:ai\s+)?(?:coding\s+|code\s+)?agent|cc_entrypoint\s*=\s*(?:cli|vscode|jetbrains|gui)|claude.?code.+issues|give feedback.+claude.?code|you are\s+.{0,30}(?:powerful\s+)?ai\s+agent|orchestration capabilities|OhMyOpenCode|<agent-identity>|<Role>|<Behavior_Instructions>`)

// codebuddyNeutralSpan replaces one matched identity span. It is a noun phrase,
// not a sentence, because it is spliced into the middle of the caller's own
// clause: "You are Claude Code, Anthropic's official CLI for Claude." becomes
// "You are a helpful coding assistant, a helpful coding assistant for Claude."
// which collapses to one readable sentence below.
const codebuddyNeutralSpan = "a helpful coding assistant"

// codebuddyNeutralTag replaces a matched scaffolding tag so the caller's XML
// structure survives with its closing pair intact.
const codebuddyNeutralTag = "<instructions>"

// codebuddyNeutralCollapse folds two neutral spans that only punctuation separates,
// so an identity claim written across several alternatives does not repeat the
// phrase. Anything else between two occurrences is caller text and is preserved.
var codebuddyNeutralCollapse = regexp.MustCompile(`(?i)` + regexp.QuoteMeta(codebuddyNeutralSpan) + `(?:[\s,;:.()\[\]<>/-]*` + regexp.QuoteMeta(codebuddyNeutralSpan) + `)+`)

const (
	neutralCodebuddyPrompt = "You are a helpful AI assistant that helps with software engineering tasks."

	// codebuddyIdentityPrompt satisfies the Intl gateway's request shape (it
	// answers 11101 to a body with no leading system message) and is used only
	// when the caller sent no system/developer text at all. It is not a mask for
	// the caller's own prompt.
	codebuddyIdentityPrompt = "You are CodeBuddy Code."

	// Rule labels name which guard fired. They reach the panel's live log view, so
	// a user asking "why did my prompt get eaten" is answerable from the UI alone.
	codebuddyRuleIdentityMarker = "coding-agent identity marker"
	codebuddyRuleTooLongFormat  = "%d chars > %d limit (raise CYRENE_CODEBUDDY_CN_MAX_SYSTEM_LEN)"
	// codebuddyRuleStrayField and codebuddyRuleShapeDrop cover the two ways text
	// can leave the instruction slot without passing through the message array:
	// a mixed-shape body carrying top-level `system`/`instructions`, and a system
	// message whose content has no text at all (image-only or scalar).
	codebuddyRuleStrayField = "top-level instruction field carried an identity marker"
	codebuddyRuleShapeDrop  = "system content had no text to preserve"

	// codebuddyCNDefaultMaxSystemLen is the CN-only length catch-all: the marker
	// regex enumerates known agents and cannot know the next one, so an over-long
	// system prompt is treated as some agent's instruction file. Upstream applies
	// no such gate to the Intl endpoint, so a long project prompt survives there.
	codebuddyCNDefaultMaxSystemLen = 2000
)

// codebuddyCNMaxSystemLen is the live catch-all threshold, overridable because the
// length rule is a heuristic that will occasionally eat a legitimate project
// prompt. Resolved per request — one Atoi beside an outbound HTTP call is free,
// and it keeps the knob observable to tests and to a restart-free override.
// internal/provider must not depend on internal/config, and CYRENE_* is already
// read in-package elsewhere (auth, system).
//
// An explicit 0 disables the catch-all entirely (the escape hatch for a caller
// whose legitimate project prompt is long); anything unparseable or negative
// falls back to the default.
func codebuddyCNMaxSystemLen() int {
	return envNonNegativeInt("CYRENE_CODEBUDDY_CN_MAX_SYSTEM_LEN", codebuddyCNDefaultMaxSystemLen)
}

func envNonNegativeInt(key string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v >= 0 {
		return v
	}
	return fallback
}

// neutralizeCodebuddySystem rewrites one system message's content when it reads
// as a third-party coding-agent identity. Tencent's WAF rejects those outright —
// 400 code 11128 "Illegal API invocation from an unapproved channel", or the
// sensitive-content refusal — and the trigger is the literal text, established by
// bisecting it upstream of this repo, not by our own guess. Content arrives either
// as a string or as typed text blocks depending on the inbound client shape, so
// the match runs on flattened text while the replacement keeps the original shape.
// maxLen <= 0 disables the length catch-all. The second return value names the
// rule that fired, or "" when the content was left alone.
func neutralizeCodebuddySystem(content any, maxLen int) (any, string) {
	// No shape whitelist: the guard keys on flattened text, so every content shape
	// JSON can produce (string, typed blocks, a bare block object) is inspected.
	// Whitelisting shapes left a bypass where an agent identity file was forwarded
	// verbatim and unlogged.
	text := messageContentText(content)
	if text == "" {
		return content, ""
	}
	// Count characters, not bytes: upstream's gate is JavaScript's string length
	// (UTF-16 units), so a Chinese prompt measured in UTF-8 bytes would be judged
	// roughly three times too long and a legitimate project prompt would be eaten.
	// Astral characters (emoji) count one here versus two upstream — the looser
	// direction, and never the trigger for the agent-file heuristic.
	//
	// The length catch-all runs first and is the ONLY branch that replaces the
	// whole slot: a >2000-char agent instruction file has no safe span to cut,
	// while a marker inside a normal prompt does.
	if maxLen > 0 {
		if n := utf8.RuneCountInString(text); n > maxLen {
			return reshapeCodebuddySystem(content, neutralCodebuddyPrompt),
				fmt.Sprintf(codebuddyRuleTooLongFormat, n, maxLen)
		}
	}
	if !agentSystemPromptRegex.MatchString(text) {
		return content, ""
	}
	return reshapeCodebuddySystem(content, neutralizeCodebuddyText(text)), codebuddyRuleIdentityMarker
}

// neutralizeCodebuddyText rewrites just the matched identity spans and returns
// the caller's remaining wording byte-for-byte. Discarding the whole message on
// a match — upstream 9router's behaviour — also throws away the project
// instructions around the marker and any per-key systemPrompt an admin appended
// into the same slot, which is the exact loss this gateway exists to avoid.
// Removing the literal is enough for the WAF: the buddy-proxy #14 bisect flipped
// 11128 to 200 by changing one character inside the phrase.
func neutralizeCodebuddyText(text string) string {
	// How much of the message the markers cover decides the strategy. Deleting the
	// spans is right for a marker inside real instructions ("You are Claude Code.
	// Be terse." keeps "Be terse."), but an identity claim written as one long
	// sentence leaves a residue ("...for Claude.") that would then stand as the
	// whole system prompt. Past two-thirds coverage the message *is* the claim, so
	// it becomes the neutral sentence instead of a fragment.
	matched := 0
	for _, span := range agentSystemPromptRegex.FindAllStringIndex(text, -1) {
		matched += span[1] - span[0]
	}
	if matched*3 >= len(text)*2 {
		return neutralCodebuddyPrompt
	}
	out := agentSystemPromptRegex.ReplaceAllStringFunc(text, func(hit string) string {
		// A tag-shaped match ("<Role>", "<agent-identity>") must be replaced by a
		// tag: splicing prose in place of an opening tag strands its closing tag
		// and corrupts the caller's XML scaffolding.
		if strings.HasPrefix(hit, "<") {
			return codebuddyNeutralTag
		}
		return codebuddyNeutralSpan
	})
	return codebuddyNeutralCollapse.ReplaceAllString(out, codebuddyNeutralSpan)
}

func reshapeCodebuddySystem(content any, text string) any {
	if _, ok := content.(string); ok {
		return text
	}
	return []any{map[string]any{"type": "text", "text": text}}
}

// sanitizeCodebuddyRequest adapts an OpenAI-shape body to what the CodeBuddy
// gateways accept. Two separate constraints, both verified upstream rather than
// assumed from our own comments:
//
//   - Shape: the Intl endpoint answers 11101 to a body without a leading system
//     message, and the CN endpoint answers 11101 to a non-stream request. Neither
//     demands any particular system text.
//   - Content: Tencent's WAF rejects a system prompt carrying a third-party
//     coding-agent identity — 400 code 11128 "Illegal API invocation from an
//     unapproved channel", or the sensitive-content refusal. The trigger is the
//     literal wording (bisected in the independent buddy-proxy project, issue
//     #14: removing one apostrophe from "Anthropic's official CLI for Claude"
//     flips 11128 to 200), so only the matched spans are rewritten and the caller
//     keeps the rest of its prompt.
//
// The pattern itself is upstream 9router's AGENT_PATTERN (open-sse/executors/
// codebuddy-intl.js) and stays byte-identical, widened only for whitespace.
// Upstream's handling (commit 3ca04639, 2026-11-08, which replaced the
// discard-everything version this repo was ported from) tests that pattern and
// then replaces the whole message; we deliberately keep the caller's surrounding
// text, because whole-message discard also eats the per-key systemPrompt an admin
// appends into the same slot. The exception is the CN length catch-all, where an
// over-long agent instruction file has no safe span to cut.
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

	rewrites := codebuddyRewrites{}
	neutralizeCodebuddyStrayFields(bodyMap, rewrites)
	rawMsgs, ok := bodyMap["messages"].([]any)
	if !ok || len(rawMsgs) == 0 {
		bodyMap["messages"] = []any{
			map[string]any{"role": "system", "content": codebuddyIdentityPrompt},
		}
		return
	}

	switch providerID {
	case "codebuddy":
		bodyMap["messages"] = codebuddyIntlMessages(rawMsgs, rewrites)
	case "codebuddy-cn":
		bodyMap["messages"] = codebuddyCNMessages(rawMsgs, rewrites)
	default:
		// An unlisted codebuddy-* sibling keeps the plain OpenAI shape: neither the
		// Intl leading-system rewrite nor the CN length gate is guessed for it. It
		// still gets marker neutralization, because that is the half that costs an
		// account. TestRegistryCoversEveryCodebuddyShape pins all three paths.
		bodyMap["messages"] = neutralizeCodebuddySystems(rawMsgs, 0, rewrites)
	}
	rewrites.log(providerID)
}

// neutralizeCodebuddyStrayFields guards the instruction slots that only exist on
// mixed-shape bodies. A client posting to /v1/chat/completions can carry a
// top-level `system` (Anthropic shape) or `instructions` (Responses shape), and
// the OpenAI->OpenAI passthrough keeps unknown keys verbatim, so an agent
// identity would reach the WAF without ever passing through the message array.
// The legitimate /v1/messages and /v1/responses adapters convert those fields
// into messages before this point, which makes this the malformed-client and
// deliberate-evasion surface only. The keys stay in place — deleting them would
// change the body shape upstream sees — only the matched spans are rewritten.
func neutralizeCodebuddyStrayFields(bodyMap map[string]any, rewrites codebuddyRewrites) {
	for _, key := range []string{"system", "instructions"} {
		text, ok := bodyMap[key].(string)
		if !ok || text == "" || !agentSystemPromptRegex.MatchString(text) {
			continue
		}
		bodyMap[key] = neutralizeCodebuddyText(text)
		rewrites.add(codebuddyRuleStrayField)
	}
}

// enforceCodebuddyCNLength re-measures the CN instruction slot after the
// guard/token-saver injections have grown it. The length catch-all in
// neutralizeCodebuddySystem runs inside the sanitizer, which must come first for
// shape reasons, but ApplyLoopGuard appends loopguard.TerminationPrompt (499
// chars) into the very message that was just measured — so a 1500-2000 char
// caller prompt would pass the gate and reach upstream over it, which is the
// rejection the gate exists to prevent. Marker neutralization is NOT re-run
// here: the injected text is ours and is already clean.
func enforceCodebuddyCNLength(providerID string, bodyMap map[string]any) {
	if providerID != "codebuddy-cn" {
		return
	}
	maxLen := codebuddyCNMaxSystemLen()
	if maxLen <= 0 {
		return
	}
	messages, _ := bodyMap["messages"].([]any)
	rewrites := codebuddyRewrites{}
	for _, m := range messages {
		msgMap, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)
		if role != "system" && role != "developer" {
			continue
		}
		text := messageContentText(msgMap["content"])
		n := utf8.RuneCountInString(text)
		if n <= maxLen {
			continue
		}
		msgMap["content"] = reshapeCodebuddySystem(msgMap["content"], neutralCodebuddyPrompt)
		rewrites.add(fmt.Sprintf(codebuddyRuleTooLongFormat, n, maxLen))
	}
	rewrites.log(providerID)
}

// codebuddyRewrites counts how often each guard fired within one request.
// Replacing client text is the one thing in this path that can silently change an
// answer, so it must never happen without a trace the operator can see in the
// panel's live log view — but the trace is aggregated, not per message: the
// messages array is client-controlled (a 10 MiB body carries ~200k minimal
// messages) and every credential retry re-runs the sanitizer, so one Warn per
// rewrite would let a single request wipe the 1000-line log ring and serialize
// that many ring memmoves against all logging in the process.
type codebuddyRewrites map[string]int

func (r codebuddyRewrites) add(rule string) {
	if rule != "" {
		r[rule]++
	}
}

// log emits one line per rule that fired, at most one per sanitize pass.
func (r codebuddyRewrites) log(providerID string) {
	rules := make([]string, 0, len(r))
	for rule := range r {
		rules = append(rules, rule)
	}
	sort.Strings(rules)
	for _, rule := range rules {
		slog.Warn("codebuddy system prompt neutralized before upstream",
			"provider", providerID, "rule", rule, "count", r[rule])
	}
}

// codebuddyIntlMessages folds the caller's own system/developer text into the
// single leading system message the Intl gateway demands, neutralizing agent
// identity markers inside it. The previous behaviour discarded that text
// unconditionally — no upstream basis, and it silently dropped the per-key
// systemPrompt an admin configures on an API key.
func codebuddyIntlMessages(rawMsgs []any, rewrites codebuddyRewrites) []any {
	var prompts []string
	out := make([]any, 0, len(rawMsgs)+1)
	for _, m := range rawMsgs {
		msgMap, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)
		if role == "system" || role == "developer" {
			content := msgMap["content"]
			text := messageContentText(content)
			if text == "" {
				// Content that exists but carries no text (image-only blocks, a
				// scalar) is about to vanish from the instruction slot. That is a
				// rewrite as much as a marker substitution is, so it gets a rule.
				if content != nil && content != "" {
					rewrites.add(codebuddyRuleShapeDrop)
				}
				continue
			}
			if agentSystemPromptRegex.MatchString(text) {
				text = neutralizeCodebuddyText(text)
				rewrites.add(codebuddyRuleIdentityMarker)
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
func codebuddyCNMessages(rawMsgs []any, rewrites codebuddyRewrites) []any {
	// Presence means "the caller already filled the instruction slot". Both roles
	// count — Intl folds developer into system, so a developer-only body must not
	// be handed a second leading slot — and the slot must carry text, because an
	// empty or null system message satisfies neither upstream nor us.
	hasSystem := false
	for _, m := range rawMsgs {
		msgMap, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)
		if role != "system" && role != "developer" {
			continue
		}
		if strings.TrimSpace(messageContentText(msgMap["content"])) != "" {
			hasSystem = true
			break
		}
	}
	messages := rawMsgs
	if !hasSystem {
		messages = append([]any{map[string]any{"role": "system", "content": codebuddyIdentityPrompt}}, rawMsgs...)
	}
	return neutralizeCodebuddySystems(messages, codebuddyCNMaxSystemLen(), rewrites)
}

// neutralizeCodebuddySystems rewrites agent-shaped system/developer messages and
// leaves everything else, including user and assistant text, untouched. Both
// roles are guarded on every shape: the WAF reads the instruction slot, and an
// OpenAI-shape body can carry it as either.
func neutralizeCodebuddySystems(rawMsgs []any, maxLen int, rewrites codebuddyRewrites) []any {
	out := make([]any, 0, len(rawMsgs))
	for _, m := range rawMsgs {
		msgMap, ok := m.(map[string]any)
		if !ok {
			out = append(out, m)
			continue
		}
		if role, _ := msgMap["role"].(string); role != "system" && role != "developer" {
			out = append(out, msgMap)
			continue
		}
		content, rule := neutralizeCodebuddySystem(msgMap["content"], maxLen)
		if rule == "" {
			out = append(out, msgMap)
			continue
		}
		rewrites.add(rule)
		clone := make(map[string]any, len(msgMap))
		maps.Copy(clone, msgMap)
		clone["content"] = content
		out = append(out, clone)
	}
	return out
}

// normalizeReasoningMaxTokens rewrites legacy max_tokens to
// max_completion_tokens for OpenAI reasoning models (gpt-5.x, gpt-6.x, o-series),
// which reject max_tokens with HTTP 400 (9router#7c42d8f8 parity).
func normalizeReasoningMaxTokens(model string, bodyMap map[string]any) {
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

func sanitizeGithubRequest(model string, bodyMap map[string]any) {
	normalizeReasoningMaxTokens(model, bodyMap)
	if eff, ok := bodyMap["reasoning_effort"].(string); ok && strings.ToLower(eff) == "none" {
		delete(bodyMap, "reasoning_effort")
	}
}

func sanitizeOpenAIRequest(model string, bodyMap map[string]any) {
	normalizeReasoningMaxTokens(model, bodyMap)
}

func isStrictOpenAISchemaProvider(providerID string) bool {
	switch providerID {
	case "openai", "github", "groq", "cerebras", "mistral":
		return true
	default:
		return false
	}
}

// sanitizeStrictOpenAIThinking converts/strips Anthropic-style top-level `thinking`
// blocks when forwarding to strict OpenAI-schema providers that reject unknown fields.
func sanitizeStrictOpenAIThinking(bodyMap map[string]any) {
	rawThinking, hasThinking := bodyMap["thinking"].(map[string]any)
	if !hasThinking {
		return
	}
	if existing, _ := bodyMap["reasoning_effort"].(string); strings.TrimSpace(existing) == "" {
		tType, _ := rawThinking["type"].(string)
		switch strings.ToLower(tType) {
		case "adaptive":
			bodyMap["reasoning_effort"] = "medium"
		case "enabled":
			budget := thinkingBudgetInt(rawThinking["budget_tokens"])
			switch {
			case budget <= 0:
				bodyMap["reasoning_effort"] = "medium"
			case budget <= 2048:
				bodyMap["reasoning_effort"] = "low"
			case budget <= 8192:
				bodyMap["reasoning_effort"] = "medium"
			default:
				bodyMap["reasoning_effort"] = "high"
			}
		}
	}
	delete(bodyMap, "thinking")
}

func thinkingBudgetInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	}
	return 0
}

func stableNamespaceUUID(key string) string {
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
			convID = stableNamespaceUUID(conversationKey)
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
