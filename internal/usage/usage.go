package usage

import (
	"encoding/json"
)

// Usage represents token usage extracted from a provider response.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	CachedTokens     int `json:"cached_tokens,omitempty"`
	ReasoningTokens  int `json:"reasoning_tokens,omitempty"`
}

// ExtractFromOpenAI extracts usage from an OpenAI-format response body.
func ExtractFromOpenAI(data []byte) Usage {
	var resp struct {
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			TotalTokens         int `json:"total_tokens"`
			PromptTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CompletionTokensDetails *struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return Usage{}
	}
	u := Usage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}
	if resp.Usage.PromptTokensDetails != nil {
		u.CachedTokens = resp.Usage.PromptTokensDetails.CachedTokens
	}
	if resp.Usage.CompletionTokensDetails != nil {
		u.ReasoningTokens = resp.Usage.CompletionTokensDetails.ReasoningTokens
	}
	return u
}

// ExtractFromClaude extracts usage from an Anthropic response body.
// Claude reports cache_read_input_tokens separately from input_tokens,
// so we fold them into prompt_tokens for canonical accounting (9router#2873).
func ExtractFromClaude(data []byte) Usage {
	var resp struct {
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return Usage{}
	}
	// Canonical: prompt includes cache read + creation (matches 9router canonicalizeUsage)
	prompt := resp.Usage.InputTokens + resp.Usage.CacheReadInputTokens + resp.Usage.CacheCreationInputTokens
	return Usage{
		PromptTokens:     prompt,
		CompletionTokens: resp.Usage.OutputTokens,
		TotalTokens:      prompt + resp.Usage.OutputTokens,
		CachedTokens:     resp.Usage.CacheReadInputTokens,
	}
}

// ExtractFromClaudeSSE extracts usage from an Anthropic message_start or message_delta event.
func ExtractFromClaudeSSE(data []byte) Usage {
	var event struct {
		Type    string `json:"type"`
		Message *struct {
			Usage struct {
				InputTokens              int `json:"input_tokens"`
				OutputTokens             int `json:"output_tokens"`
				CacheReadInputTokens     int `json:"cache_read_input_tokens"`
				CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			} `json:"usage"`
		} `json:"message"`
		Usage *struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return Usage{}
	}
	if event.Type == "message_start" && event.Message != nil {
		u := event.Message.Usage
		prompt := u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
		return Usage{
			PromptTokens:     prompt,
			CompletionTokens: u.OutputTokens,
			TotalTokens:      prompt + u.OutputTokens,
			CachedTokens:     u.CacheReadInputTokens,
		}
	}
	if (event.Type == "message_delta" || event.Type == "") && event.Usage != nil {
		u := event.Usage
		prompt := u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
		return Usage{
			PromptTokens:     prompt,
			CompletionTokens: u.OutputTokens,
			TotalTokens:      prompt + u.OutputTokens,
			CachedTokens:     u.CacheReadInputTokens,
		}
	}
	return Usage{}
}

// ExtractFromGemini extracts usage from a Gemini response body.
func ExtractFromGemini(data []byte) Usage {
	var resp struct {
		UsageMetadata struct {
			PromptTokenCount        int `json:"promptTokenCount"`
			CandidatesTokenCount    int `json:"candidatesTokenCount"`
			TotalTokenCount         int `json:"totalTokenCount"`
			CachedContentTokenCount int `json:"cachedContentTokenCount"`
			ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return Usage{}
	}
	u := Usage{
		PromptTokens:     resp.UsageMetadata.PromptTokenCount,
		CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
		TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		CachedTokens:     resp.UsageMetadata.CachedContentTokenCount,
		ReasoningTokens:  resp.UsageMetadata.ThoughtsTokenCount,
	}
	// Gemini includes thoughts in completion
	if resp.UsageMetadata.ThoughtsTokenCount > 0 {
		u.CompletionTokens += resp.UsageMetadata.ThoughtsTokenCount
	}
	return u
}

// ExtractFromSSELine extracts usage from an OpenAI SSE chunk (final chunk often has usage).
func ExtractFromSSELine(data []byte) Usage {
	var chunk struct {
		Usage *struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			TotalTokens         int `json:"total_tokens"`
			PromptTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CompletionTokensDetails *struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &chunk); err != nil || chunk.Usage == nil {
		return Usage{}
	}
	u := Usage{
		PromptTokens:     chunk.Usage.PromptTokens,
		CompletionTokens: chunk.Usage.CompletionTokens,
		TotalTokens:      chunk.Usage.TotalTokens,
	}
	if chunk.Usage.PromptTokensDetails != nil {
		u.CachedTokens = chunk.Usage.PromptTokensDetails.CachedTokens
	}
	if chunk.Usage.CompletionTokensDetails != nil {
		u.ReasoningTokens = chunk.Usage.CompletionTokensDetails.ReasoningTokens
	}
	return u
}
