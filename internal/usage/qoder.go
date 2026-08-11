// qoder.go fetches Qoder quota usage — port of 9router
// services/usage/misc.js getQoderUsage + the PAT resolution in usage.js
// (9router@d433c0b2): PAT (pt-...) connections are exchanged for a job token
// before the quota endpoint accepts them.

package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/provider"
)

const qoderQuotaUsageURL = "https://openapi.qoder.sh/api/v2/quota/usage"

func fetchQoder(ctx context.Context, client *http.Client, c QuotaCredentials) QuotaResult {
	token := strings.TrimSpace(c.AccessToken)
	if token == "" {
		token = strings.TrimSpace(c.APIKey)
	}
	if token == "" {
		return QuotaResult{Message: "Qoder usage unavailable: no access token"}
	}

	// PAT (pt-...) connections must be exchanged to a job token first.
	if provider.IsQoderPAT(token) {
		resolved, err := provider.ResolveQoderCredential(token, "", client)
		if err != nil {
			return QuotaResult{Message: "Qoder PAT exchange failed: " + err.Error()}
		}
		token = resolved.AccessToken
	}

	url := endpoint(qoderQuotaUsageURL, c.BaseURL)
	status, data, raw, err := getJSON(ctx, client, url, token)
	if err != nil {
		return QuotaResult{Message: "Qoder connected. Unable to fetch usage: " + err.Error()}
	}
	if status != http.StatusOK {
		return QuotaResult{Message: fmt.Sprintf("Qoder connected. Usage fetch returned %d.", status)}
	}
	if len(raw) == 0 {
		return QuotaResult{Message: "Qoder connected. Usage response was not JSON."}
	}
	if data == nil {
		if err := json.Unmarshal([]byte(raw), &data); err != nil {
			return QuotaResult{Message: "Qoder connected. Usage response was not JSON."}
		}
	}

	// Quota records live under userQuota / orgResourcePackage; the absolute
	// reset timestamp (expiresAt ms) is surfaced on every record.
	userQuota, _ := data["userQuota"].(map[string]any)
	orgQuota, _ := data["orgResourcePackage"].(map[string]any)

	resetAt := ""
	if expMs := num(data["expiresAt"]); expMs > 0 {
		resetAt = unixToISO(expMs)
	}

	quotas := map[string]Quota{}
	addQoderQuota := func(name string, q map[string]any) {
		if q == nil {
			return
		}
		total := num(q["total"])
		used := num(q["used"])
		remaining := num(q["remaining"])
		if total <= 0 && used <= 0 && remaining <= 0 {
			return
		}
		unit, _ := q["unit"].(string)
		if unit == "" {
			unit = "credits"
		}
		remainingPct := 0.0
		if total > 0 {
			remainingPct = remaining / total * 100
			if remainingPct < 0 {
				remainingPct = 0
			}
			if remainingPct > 100 {
				remainingPct = 100
			}
		}
		quotas[name] = Quota{
			Used:                used,
			Total:               total,
			Remaining:           remaining,
			RemainingPercentage: remainingPct,
			ResetAt:             resetAt,
			Unit:                unit,
		}
	}
	addQoderQuota("user", userQuota)
	addQoderQuota("organization", orgQuota)

	if len(quotas) == 0 {
		return QuotaResult{Message: "Qoder connected. No quota data was returned."}
	}

	plan := "Qoder"
	if pct := num(data["totalUsagePercentage"]); pct > 0 {
		plan = fmt.Sprintf("Qoder (%.0f%% used)", pct)
	}
	if exceeded, _ := data["isQuotaExceeded"].(bool); exceeded {
		plan = "Qoder (Quota Exceeded)"
	}
	return QuotaResult{Plan: plan, Quotas: quotas}
}
