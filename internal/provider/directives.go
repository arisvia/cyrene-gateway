package provider

import (
	"encoding/json"
	"regexp"
	"strings"
)

// DirectiveResult holds the overrides extracted from in-turn prompt directives.
type DirectiveResult struct {
	Model           string
	ReasoningEffort string
	HasOverrides    bool
}

var (
	// Matches directives like @cg-effort:high, @ocr-effort:low, @cg-model:gpt-4o, @ocr-effort-up, @ocr-effort-down
	directiveEffortValueRegex = regexp.MustCompile(`(?i)@(?:cg|ocr)-effort[:=]([a-zA-Z0-9_-]+)`)
	directiveModelValueRegex  = regexp.MustCompile(`(?i)@(?:cg|ocr)-model[:=]([a-zA-Z0-9_\-./]+)`)
	directiveEffortUpRegex    = regexp.MustCompile(`(?i)@(?:cg|ocr)-effort-up\b`)
	directiveEffortDownRegex  = regexp.MustCompile(`(?i)@(?:cg|ocr)-effort-down\b`)
)

// ExtractAndScrubDirectives parses and removes @cg-* and @ocr-* directives from text.
func ExtractAndScrubDirectives(text string) (string, DirectiveResult) {
	var res DirectiveResult
	if !strings.Contains(text, "@cg-") && !strings.Contains(text, "@ocr-") &&
		!strings.Contains(text, "@CG-") && !strings.Contains(text, "@OCR-") {
		return text, res
	}

	cleaned := text

	// 1. Model directive
	if matches := directiveModelValueRegex.FindStringSubmatch(cleaned); len(matches) > 1 {
		res.Model = strings.TrimSpace(matches[1])
		res.HasOverrides = true
		cleaned = directiveModelValueRegex.ReplaceAllString(cleaned, "")
	}

	// 2. Effort directive with explicit value
	if matches := directiveEffortValueRegex.FindStringSubmatch(cleaned); len(matches) > 1 {
		val := strings.ToLower(strings.TrimSpace(matches[1]))
		switch val {
		case "low", "medium", "high", "max", "adaptive", "none", "off":
			res.ReasoningEffort = val
			res.HasOverrides = true
		}
		cleaned = directiveEffortValueRegex.ReplaceAllString(cleaned, "")
	}

	// 3. Step up / down directives
	if directiveEffortUpRegex.MatchString(cleaned) {
		res.ReasoningEffort = "high"
		res.HasOverrides = true
		cleaned = directiveEffortUpRegex.ReplaceAllString(cleaned, "")
	}
	if directiveEffortDownRegex.MatchString(cleaned) {
		res.ReasoningEffort = "low"
		res.HasOverrides = true
		cleaned = directiveEffortDownRegex.ReplaceAllString(cleaned, "")
	}

	// Clean up extra whitespace left by directive removal
	cleaned = strings.TrimSpace(cleaned)
	return cleaned, res
}

// ScrubDirectivesFromMessages inspects the messages list (scanning the last user message),
// extracts any inline directives, removes them from the message content, and returns the overrides
// along with the updated messages JSON or payload.
func ScrubDirectivesFromOpenAIBody(bodyMap map[string]any) DirectiveResult {
	var combined DirectiveResult
	msgs, ok := bodyMap["messages"].([]any)
	if !ok || len(msgs) == 0 {
		return combined
	}

	// Find the last user message
	for i := len(msgs) - 1; i >= 0; i-- {
		msg, ok := msgs[i].(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if strings.ToLower(role) != "user" {
			continue
		}

		content := msg["content"]
		switch c := content.(type) {
		case string:
			cleaned, dir := ExtractAndScrubDirectives(c)
			if dir.HasOverrides {
				msg["content"] = cleaned
				combined = dir
			}
		case []any:
			// Content blocks (e.g. text/image blocks)
			for _, item := range c {
				block, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if t, ok := block["text"].(string); ok {
					cleaned, dir := ExtractAndScrubDirectives(t)
					if dir.HasOverrides {
						block["text"] = cleaned
						combined = dir
					}
				}
			}
		}

		if combined.HasOverrides {
			break
		}
	}

	if combined.Model != "" {
		bodyMap["model"] = combined.Model
	}
	if combined.ReasoningEffort != "" {
		bodyMap["reasoning_effort"] = combined.ReasoningEffort
	}

	return combined
}

// ScrubDirectivesFromRawJSON scrubs directives from raw JSON payload directly.
func ScrubDirectivesFromRawJSON(rawBody []byte) ([]byte, DirectiveResult, error) {
	var bodyMap map[string]any
	if err := json.Unmarshal(rawBody, &bodyMap); err != nil {
		return rawBody, DirectiveResult{}, err
	}
	res := ScrubDirectivesFromOpenAIBody(bodyMap)
	if !res.HasOverrides {
		return rawBody, res, nil
	}
	updated, err := json.Marshal(bodyMap)
	if err != nil {
		return rawBody, res, err
	}
	return updated, res, nil
}
