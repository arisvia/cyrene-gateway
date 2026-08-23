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
	ModelStr     string
	ModelInfo    model.ModelInfo
	ProviderInfo ProviderInfo
	Connection   *model.ProviderConnection
}

// ExecutionRequest holds the normalized request payload and metadata.
type ExecutionRequest struct {
	Model      string
	Stream     bool
	RawBody    []byte
	Messages   []map[string]any
	HasTools   bool
	SaveTokens bool
}

// ExecutionResult is the outcome of executing against an upstream provider.
type ExecutionResult struct {
	Response       *http.Response
	TargetFormat   translator.Format
	ModelInfo      model.ModelInfo
	Connection     *model.ProviderConnection
	StatusCode     int
	ErrorBody      []byte
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
	}

	transport := ResolveTransport(providerInfo, baseURL, effectiveAPIType, conn)

	var bodyMap map[string]any
	if err := json.Unmarshal(req.RawBody, &bodyMap); err != nil {
		bodyMap = make(map[string]any)
	}
	bodyMap["model"] = modelInfo.Model

	formatStr := "openai"
	if targetFormat != translator.FormatOpenAI {
		formatStr = string(targetFormat)
	}

	// Loop guard
	if req.Messages != nil {
		// Convert to loopguard messages
		lgMsgs := make([]loopguard.Message, 0, len(req.Messages))
		for _, m := range req.Messages {
			role, _ := m["role"].(string)
			content, _ := m["content"].(string)
			lgMsgs = append(lgMsgs, loopguard.Message{Role: role, Content: json.RawMessage(fmt.Sprintf("%q", content))})
		}
		if lgRes := loopguard.DetectLoop(lgMsgs); lgRes.Detected {
			loopguard.InjectLoopHint(bodyMap, formatStr, lgRes.Hint)
		}
	}

	// Termination prompt
	if req.HasTools {
		loopguard.InjectTerminationPrompt(bodyMap, formatStr)
	}

	var bodyBytes []byte
	var err error

	if targetFormat == translator.FormatOpenAI {
		ClampMaxTokens(modelInfo.Provider, modelInfo.Model, bodyMap)
		if applyTokenSaverFn != nil {
			applyTokenSaverFn(bodyMap, "openai")
		}
		bodyBytes, err = json.Marshal(bodyMap)
	} else {
		translated, trErr := translator.TranslateRequest(targetFormat, modelInfo.Model, bodyMap, req.Stream)
		if trErr != nil {
			return nil, "", fmt.Errorf("translation failed: %w", trErr)
		}
		ClampMaxTokens(modelInfo.Provider, modelInfo.Model, translated)
		if applyTokenSaverFn != nil {
			applyTokenSaverFn(translated, string(targetFormat))
		}
		bodyBytes, err = json.Marshal(translated)
	}

	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal request: %w", err)
	}

	targetURL := BuildTransportURL(transport, modelInfo.Model, req.Stream)
	upstreamReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create upstream request: %w", err)
	}

	for k, v := range transport.Headers {
		upstreamReq.Header.Set(k, v)
	}
	creds := Credentials{
		APIKey:               conn.Data.APIKey,
		AccessToken:          conn.Data.AccessToken,
		ProviderSpecificData: conn.Data.ProviderSpecificData,
	}
	ApplyAuth(upstreamReq, transport, creds)
	if providerInfo.NoAuth && creds.APIKey == "" && creds.AccessToken == "" {
		upstreamReq.Header.Set("Authorization", "Bearer public")
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	if req.Stream {
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

	// Read upstream error body
	errBody, _ := io.ReadAll(resp.Body)
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
