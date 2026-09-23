// antigravity.go extracts per-model quota buckets from Cloud Code Assist
// /v1internal:fetchAvailableModels endpoint. Ported from 9router and picoclaw.

package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
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

type AntigravityQuotaSummaryResponse struct {
	Groups []struct {
		DisplayName string `json:"displayName"`
		Description string `json:"description"`
		Buckets     []struct {
			BucketID          string   `json:"bucketId"`
			Window            string   `json:"window"`
			RemainingFraction *float64 `json:"remainingFraction"`
			ResetTime         string   `json:"resetTime"`
			DisplayName       string   `json:"displayName"`
			Description       string   `json:"description"`
		} `json:"buckets"`
	} `json:"groups"`
}

func fetchAntigravity(ctx context.Context, client *http.Client, c QuotaCredentials) QuotaResult {
	token := strings.TrimSpace(c.AccessToken)
	if token == "" {
		token = strings.TrimSpace(c.APIKey)
	}
	if token == "" {
		return QuotaResult{Message: "Antigravity credentials missing"}
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

	// 1. First attempt: retrieveUserQuotaSummary (provides weekly + 5h grouped pools)
	for _, ep := range endpoints {
		summaryURL := ep + "/v1internal:retrieveUserQuotaSummary"
		req, err := http.NewRequestWithContext(ctx, "POST", summaryURL, bytes.NewReader(bodyBytes))
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", provider.AntigravityUserAgent)

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err == nil && resp.StatusCode == http.StatusOK {
			var summary AntigravityQuotaSummaryResponse
			if err := json.Unmarshal(respBytes, &summary); err == nil && len(summary.Groups) > 0 {
				quotas := clusterAntigravitySummary(summary)
				if len(quotas) > 0 {
					return QuotaResult{
						Plan:   "Google Code Assist",
						Quotas: quotas,
					}
				}
			}
		}
	}

	// 2. Second attempt: fetchAvailableModels (standard per-model quota pooling fallback)
	for _, ep := range endpoints {
		url := ep + "/v1internal:fetchAvailableModels"
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			continue
		}

		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", provider.AntigravityUserAgent)

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
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
	resetSoon   bool
}

func clusterAntigravityQuotas(parsed AntigravityModelsResponse) map[string]Quota {
	if len(parsed.Models) == 0 {
		return nil
	}

	var geminiModels []modelQuotaRaw
	var claudeGptModels []modelQuotaRaw
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

		resetAt := ""
		resetSoon := false
		rawReset := strings.TrimSpace(m.QuotaInfo.ResetTime)
		if rawReset != "" {
			if t, err := time.Parse(time.RFC3339Nano, rawReset); err == nil {
				resetAt = t.UTC().Format(time.RFC3339)
				if time.Until(t) <= 0 {
					resetSoon = true
				}
			} else if t, err := time.Parse(time.RFC3339, rawReset); err == nil {
				resetAt = t.UTC().Format(time.RFC3339)
				if time.Until(t) <= 0 {
					resetSoon = true
				}
			} else if rawReset == "即将重置" || strings.EqualFold(rawReset, "reset soon") || strings.EqualFold(rawReset, "resetting soon") {
				resetSoon = true
			}
		}

		raw := modelQuotaRaw{
			id:          id,
			displayName: dispName,
			remFrac:     remFrac,
			resetAt:     resetAt,
			resetSoon:   resetSoon,
		}

		lowerID := strings.ToLower(id)
		if strings.HasPrefix(lowerID, "gemini-") {
			geminiModels = append(geminiModels, raw)
		} else if strings.HasPrefix(lowerID, "claude-") || strings.HasPrefix(lowerID, "gpt-") {
			// Claude and GPT-OSS share the exact same quota pool in Antigravity
			claudeGptModels = append(claudeGptModels, raw)
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
		rem := math.Round(rep.remFrac * total)
		quotas["gemini"] = Quota{
			DisplayName:         "Gemini (Flash / Pro / Image / Search)",
			Total:               total,
			Used:                total - rem,
			Remaining:           rem,
			RemainingPercentage: rep.remFrac * 100.0,
			ResetAt:             rep.resetAt,
			ResetSoon:           rep.resetSoon,
			Unit:                "requests",
		}
	}

	// 2. Aggregate Claude & GPT shared models pool (worst-case remainingFraction wins)
	if len(claudeGptModels) > 0 {
		rep := claudeGptModels[0]
		for _, m := range claudeGptModels[1:] {
			if m.remFrac < rep.remFrac {
				rep = m
			}
		}
		rem := math.Round(rep.remFrac * total)
		quotas["claude"] = Quota{
			DisplayName:         "Claude & GPT (Shared)",
			Total:               total,
			Used:                total - rem,
			Remaining:           rem,
			RemainingPercentage: rep.remFrac * 100.0,
			ResetAt:             rep.resetAt,
			ResetSoon:           rep.resetSoon,
			Unit:                "requests",
		}
	}
	// 3. Image generation models
	for _, m := range imageModels {
		rem := math.Round(m.remFrac * total)
		quotas[m.id] = Quota{
			DisplayName:         m.displayName,
			Total:               total,
			Used:                total - rem,
			Remaining:           rem,
			RemainingPercentage: m.remFrac * 100.0,
			ResetAt:             m.resetAt,
			ResetSoon:           m.resetSoon,
			Unit:                "requests",
		}
	}

	// 4. Other standalone models (e.g. gpt-oss-120b-medium)
	for _, m := range otherModels {
		rem := math.Round(m.remFrac * total)
		quotas[m.id] = Quota{
			DisplayName:         m.displayName,
			Total:               total,
			Used:                total - rem,
			Remaining:           rem,
			RemainingPercentage: m.remFrac * 100.0,
			ResetAt:             m.resetAt,
			ResetSoon:           m.resetSoon,
			Unit:                "requests",
		}
	}

	return quotas
}

func clusterAntigravitySummary(summary AntigravityQuotaSummaryResponse) map[string]Quota {
	quotas := make(map[string]Quota)
	const total = 1000.0

	for _, g := range summary.Groups {
		groupName := strings.TrimSpace(g.DisplayName)
		lowerGroup := strings.ToLower(groupName)

		baseKey := "other"
		baseDisplay := groupName
		if strings.Contains(lowerGroup, "gemini") {
			baseKey = "gemini"
			baseDisplay = "Gemini (Search / Image)"
		} else if strings.Contains(lowerGroup, "claude") || strings.Contains(lowerGroup, "gpt") {
			baseKey = "claude"
			baseDisplay = "Claude & GPT"
		}

		for _, b := range g.Buckets {
			remFrac := 1.0
			if b.RemainingFraction != nil {
				remFrac = *b.RemainingFraction
			}
			rem := math.Round(remFrac * total)

			resetAt := ""
			resetSoon := false
			rawReset := strings.TrimSpace(b.ResetTime)
			if rawReset != "" {
				if t, err := time.Parse(time.RFC3339Nano, rawReset); err == nil {
					resetAt = t.UTC().Format(time.RFC3339)
					if time.Until(t) <= 0 {
						resetSoon = true
					}
				} else if t, err := time.Parse(time.RFC3339, rawReset); err == nil {
					resetAt = t.UTC().Format(time.RFC3339)
					if time.Until(t) <= 0 {
						resetSoon = true
					}
				} else if rawReset == "即将重置" || strings.EqualFold(rawReset, "reset soon") || strings.EqualFold(rawReset, "resetting soon") {
					resetSoon = true
				}
			}

			window := strings.ToLower(strings.TrimSpace(b.Window))
			bucketID := strings.ToLower(strings.TrimSpace(b.BucketID))
			isWeekly := window == "7d" || window == "weekly" || strings.Contains(bucketID, "weekly") || strings.Contains(strings.ToLower(b.DisplayName), "week")

			key := baseKey
			label := baseDisplay
			if isWeekly {
				key = baseKey + "_weekly"
				label = baseDisplay + " (Weekly)"
			} else {
				key = baseKey + "_session"
				label = baseDisplay + " (5h Rolling)"
			}

			quotas[key] = Quota{
				DisplayName:         label,
				Total:               total,
				Used:                total - rem,
				Remaining:           rem,
				RemainingPercentage: remFrac * 100.0,
				ResetAt:             resetAt,
				ResetSoon:           resetSoon,
				Unit:                "requests",
			}
		}
	}
	return quotas
}
