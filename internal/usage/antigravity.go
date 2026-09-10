// antigravity.go extracts per-model quota buckets from Cloud Code Assist
// /v1internal:fetchAvailableModels endpoint. Ported from 9router and picoclaw.

package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/provider"
)

type AntigravityModelsResponse struct {
	Models map[string]struct {
		DisplayName string `json:"displayName"`
		QuotaInfo   *struct {
			RemainingFraction *float64 `json:"remainingFraction"`
			ResetTime         string   `json:"resetTime"`
			IsExhausted       bool     `json:"isExhausted"`
		} `json:"quotaInfo"`
	} `json:"models"`
}

func fetchAntigravity(ctx context.Context, client *http.Client, c QuotaCredentials) QuotaResult {
	if c.AccessToken == "" {
		return QuotaResult{Message: "Antigravity OAuth access token is missing"}
	}

	endpoints := []string{provider.AntigravityBaseURL, provider.AntigravityDailyURL}
	if c.BaseURL != "" {
		endpoints = []string{c.BaseURL}
	}

	body := map[string]any{}
	if c.ProjectID != "" {
		body["project"] = c.ProjectID
	}
	bodyBytes, _ := json.Marshal(body)

	for _, ep := range endpoints {
		url := ep + "/v1internal:fetchAvailableModels"
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			continue
		}

		req.Header.Set("Authorization", "Bearer "+c.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", provider.AntigravityUserAgent)
		req.Header.Set("X-Goog-Api-Client", provider.AntigravityXGoogClient)
		req.Header.Set("Client-Metadata", provider.AntigravityMetadata)

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		respBytes, err := io.ReadAll(resp.Body)
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		var parsed AntigravityModelsResponse
		if err := json.Unmarshal(respBytes, &parsed); err != nil {
			continue
		}
		quotas := clusterAntigravityQuotas(parsed)
		if len(quotas) > 0 {
			return QuotaResult{
				Plan:   "Google Code Assist",
				Quotas: quotas,
			}
		}
	}

	return QuotaResult{
		Message: "Antigravity quota endpoint currently returned no active quota buckets",
	}
}

type modelQuotaRaw struct {
	id          string
	displayName string
	remFrac     float64
	resetAt     string
}

func clusterAntigravityQuotas(parsed AntigravityModelsResponse) map[string]Quota {
	if len(parsed.Models) == 0 {
		return nil
	}

	var geminiModels []modelQuotaRaw
	var claudeModels []modelQuotaRaw
	var imageModels []modelQuotaRaw
	var otherModels []modelQuotaRaw

	for id, m := range parsed.Models {
		if m.QuotaInfo == nil {
			continue
		}
		// Filter out internal Google test/telemetry endpoints and completion-only tab anchors
		if strings.HasPrefix(id, "chat_") || strings.HasPrefix(id, "tab_") || strings.HasPrefix(id, "rev_") {
			continue
		}

		dispName := m.DisplayName
		if dispName == "" {
			dispName = id
		}

		remFrac := 1.0
		if m.QuotaInfo.RemainingFraction != nil {
			remFrac = *m.QuotaInfo.RemainingFraction
		}
		if m.QuotaInfo.IsExhausted {
			remFrac = 0.0
		}

		resetAt := m.QuotaInfo.ResetTime
		if resetAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, resetAt); err == nil {
				resetAt = t.UTC().Format(time.RFC3339)
			}
		}

		raw := modelQuotaRaw{
			id:          id,
			displayName: dispName,
			remFrac:     remFrac,
			resetAt:     resetAt,
		}

		lowerID := strings.ToLower(id)
		if strings.HasPrefix(lowerID, "gemini-") {
			geminiModels = append(geminiModels, raw)
		} else if strings.HasPrefix(lowerID, "claude-") {
			claudeModels = append(claudeModels, raw)
		} else if strings.Contains(lowerID, "image") || strings.HasPrefix(lowerID, "imagen-") {
			imageModels = append(imageModels, raw)
		} else {
			otherModels = append(otherModels, raw)
		}
	}

	quotas := make(map[string]Quota)
	const total = 1000.0

	// 1. Aggregate Gemini text models pool (worst-case remainingFraction wins)
	if len(geminiModels) > 0 {
		rep := geminiModels[0]
		for _, m := range geminiModels[1:] {
			if m.remFrac < rep.remFrac {
				rep = m
			}
		}
		rem := rep.remFrac * total
		quotas["gemini"] = Quota{
			DisplayName:         "Gemini (Flash / Pro / Image)",
			Total:               total,
			Used:                total - rem,
			Remaining:           rem,
			RemainingPercentage: rep.remFrac * 100.0,
			ResetAt:             rep.resetAt,
			Unit:                "requests",
		}
	}

	// 2. Aggregate Claude models pool
	if len(claudeModels) > 0 {
		rep := claudeModels[0]
		for _, m := range claudeModels[1:] {
			if m.remFrac < rep.remFrac {
				rep = m
			}
		}
		rem := rep.remFrac * total
		quotas["claude"] = Quota{
			DisplayName:         "Claude (Sonnet / Opus)",
			Total:               total,
			Used:                total - rem,
			Remaining:           rem,
			RemainingPercentage: rep.remFrac * 100.0,
			ResetAt:             rep.resetAt,
			Unit:                "requests",
		}
	}

	// 3. Image generation models
	for _, m := range imageModels {
		rem := m.remFrac * total
		quotas[m.id] = Quota{
			DisplayName:         m.displayName,
			Total:               total,
			Used:                total - rem,
			Remaining:           rem,
			RemainingPercentage: m.remFrac * 100.0,
			ResetAt:             m.resetAt,
			Unit:                "requests",
		}
	}

	// 4. Other standalone models (e.g. gpt-oss-120b-medium)
	for _, m := range otherModels {
		rem := m.remFrac * total
		quotas[m.id] = Quota{
			DisplayName:         m.displayName,
			Total:               total,
			Used:                total - rem,
			Remaining:           rem,
			RemainingPercentage: m.remFrac * 100.0,
			ResetAt:             m.resetAt,
			Unit:                "requests",
		}
	}

	return quotas
}
