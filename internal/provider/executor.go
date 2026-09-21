package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/arisvia/cyrene-gateway/internal/loopguard"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/translator"
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

	var history struct {
		Messages []loopguard.Message `json:"messages"`
	}
	if err := json.Unmarshal(req.RawBody, &history); err != nil {
		return nil, "", fmt.Errorf("invalid messages: %w", err)
	}
	if result := loopguard.DetectLoop(history.Messages); result.Detected {
		loopguard.InjectLoopHint(bodyMap, "openai", result.Hint)
	}
	if req.HasTools {
		loopguard.InjectTerminationPrompt(bodyMap, "openai")
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
	ClampMaxTokens(modelInfo.Provider, modelInfo.Model, bodyMap)
	if applyTokenSaverFn != nil {
		applyTokenSaverFn(bodyMap, "openai")
	}
	translated, err := translator.TranslateRequest(targetFormat, modelInfo.Model, bodyMap, upstreamStream)
	if err != nil {
		return nil, "", fmt.Errorf("translation failed: %w", err)
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
