// opencode.go extracts real usage/quota metrics from OpenCode Go API.
// Ported from 9router open-sse/services/usage/opencode-go.js.

package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const opencodeUsageURL = "https://opencode.ai/zen/go/v1/usage"

type opencodeQuotaWindow struct {
	Percent  any `json:"percent"`
	ResetsAt any `json:"resetsAt"`
}

type opencodeUsageResponse struct {
	Usage map[string]opencodeQuotaWindow `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func fetchOpenCode(ctx context.Context, client *http.Client, creds QuotaCredentials) QuotaResult {
	apiKey := strings.TrimSpace(creds.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(creds.AccessToken)
	}
	if apiKey == "" {
		return QuotaResult{Message: "OpenCode API key not configured"}
	}

	url := opencodeUsageURL
	if creds.BaseURL != "" {
		base := strings.TrimRight(creds.BaseURL, "/")
		base = strings.TrimSuffix(base, "/chat/completions")
		url = base + "/usage"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return QuotaResult{Message: fmt.Sprintf("OpenCode usage error: %v", err)}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "opencode")
	req.Header.Set("x-opencode-client", "desktop")

	resp, err := quotaHTTPClient(client).Do(req)
	if err != nil {
		return QuotaResult{Message: fmt.Sprintf("OpenCode usage request failed: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return QuotaResult{
			Plan:    "OpenCode",
			Message: "OpenCode authentication failed, please verify API key",
		}
	}
	if resp.StatusCode == http.StatusForbidden {
		return QuotaResult{
			Plan:    "OpenCode Zen",
			Message: "OpenCode Zen plan does not expose quota usage endpoint (Go subscription required)",
		}
	}
	if resp.StatusCode != http.StatusOK {
		return QuotaResult{
			Plan:    "OpenCode",
			Message: fmt.Sprintf("OpenCode usage request failed with status %d", resp.StatusCode),
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return QuotaResult{Message: fmt.Sprintf("OpenCode usage read error: %v", err)}
	}

	var data opencodeUsageResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return QuotaResult{Message: fmt.Sprintf("OpenCode usage JSON parse error: %v", err)}
	}

	if len(data.Usage) == 0 {
		return QuotaResult{
			Plan:    "OpenCode",
			Message: "OpenCode usage response contains no quota records",
		}
	}

	quotas := make(map[string]Quota)
	periodNames := map[string]string{
		"rolling": "Rolling",
		"weekly":  "Weekly",
		"monthly": "Monthly",
	}

	for period, displayName := range periodNames {
		window, ok := data.Usage[period]
		if !ok {
			continue
		}
		pVal := num(window.Percent)
		if pVal < 0 {
			pVal = 0
		}
		if pVal > 100 {
			pVal = 100
		}
		used := pVal
		remaining := 100 - used
		quotas[displayName] = Quota{
			Used:                used,
			Total:               100,
			Remaining:           remaining,
			RemainingPercentage: remaining,
			ResetAt:             parseResetTime(window.ResetsAt),
			Unit:                "%",
			DisplayName:         displayName,
		}
	}

	if len(quotas) == 0 {
		return QuotaResult{
			Plan:    "OpenCode",
			Message: "OpenCode usage response contains no recognizable quota metrics",
		}
	}
	return QuotaResult{
		Plan:   "OpenCode Go",
		Quotas: quotas,
	}
}
