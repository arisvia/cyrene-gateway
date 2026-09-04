// antigravity.go extracts per-model quota buckets from Cloud Code Assist
// /v1internal:fetchAvailableModels endpoint. Ported from 9router and picoclaw.

package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	antigravityBaseURL   = "https://cloudcode-pa.googleapis.com"
	antigravityDailyURL  = "https://daily-cloudcode-pa.sandbox.googleapis.com"
	antigravityUserAgent = "antigravity/1.15.8 windows/amd64"
	antigravityXGoog     = "google-cloud-sdk vscode_cloudshelleditor/0.1"
	antigravityMetadata  = `{"ideType":"ANTIGRAVITY","platform":"PLATFORM_UNSPECIFIED","pluginType":"GEMINI"}`
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

	endpoints := []string{antigravityBaseURL, antigravityDailyURL}
	if c.BaseURL != "" {
		endpoints = []string{c.BaseURL}
	}

	body := map[string]any{
		"metadata": json.RawMessage(antigravityMetadata),
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
		req.Header.Set("User-Agent", antigravityUserAgent)
		req.Header.Set("X-Goog-Api-Client", antigravityXGoog)
		req.Header.Set("Client-Metadata", antigravityMetadata)

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

		quotas := make(map[string]Quota)
		for id, m := range parsed.Models {
			if m.QuotaInfo == nil {
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

			// Format: 1,000 total bucket base
			total := 1000.0
			rem := remFrac * total
			used := total - rem

			pct := remFrac * 100.0

			resetAt := m.QuotaInfo.ResetTime
			if resetAt != "" {
				if t, err := time.Parse(time.RFC3339Nano, resetAt); err == nil {
					diff := time.Until(t)
					if diff > 0 {
						hours := int(diff.Hours())
						mins := int(diff.Minutes()) % 60
						if hours > 0 {
							resetAt = fmt.Sprintf("in %dh %dm", hours, mins)
						} else {
							resetAt = fmt.Sprintf("in %dm", mins)
						}
					}
				}
			}

			quotas[id] = Quota{
				DisplayName:         dispName,
				Total:               total,
				Used:                used,
				Remaining:           rem,
				RemainingPercentage: pct,
				ResetAt:             resetAt,
				Unit:                "requests",
			}
		}

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
