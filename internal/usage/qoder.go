// qoder.go fetches Qoder quota usage — port of 9router
// services/usage/misc.js getQoderUsage + the PAT resolution in usage.js
// (9router@d433c0b2): PAT (pt-...) connections are exchanged for a job token
// before the quota endpoint accepts them.

package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/provider"
)

const qoderQuotaUsageURL = "https://openapi.qoder.sh/api/v2/quota/usage"

func qoderQuotaURL(baseURL string) string {
	if baseURL != "" {
		lower := strings.ToLower(baseURL)
		if strings.Contains(lower, "127.0.0.1") || strings.Contains(lower, "localhost") || strings.Contains(lower, "openapi.qoder.sh") {
			return endpoint(qoderQuotaUsageURL, baseURL)
		}
	}
	return qoderQuotaUsageURL
}

func getQoderJSON(ctx context.Context, client *http.Client, url, token string) (int, map[string]any, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "qodercli/1.0.0")
	req.Header.Set("Cosy-Version", "1.26.0")
	req.Header.Set("Cosy-ClientType", "5")
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out, string(raw), nil
}

func fetchQoder(ctx context.Context, client *http.Client, c QuotaCredentials) QuotaResult {
	token := strings.TrimSpace(c.AccessToken)
	if token == "" {
		token = strings.TrimSpace(c.APIKey)
	}
	if token == "" {
		return QuotaResult{Message: "Qoder usage unavailable: no access token"}
	}

	accountID := ""
	// PAT (pt-...) connections must be exchanged to a job token first.
	if provider.IsQoderPAT(token) {
		resolved, err := provider.ResolveQoderCredential(token, "", client)
		if err != nil {
			return QuotaResult{
				Plan:    "Qoder",
				Message: "Qoder PAT exchange failed (token expired or unauthorized): " + err.Error(),
			}
		}
		token = resolved.AccessToken
		accountID = resolved.UserID
	}

	url := qoderQuotaURL(c.BaseURL)
	status, data, raw, err := getQoderJSON(ctx, client, url, token)
	if err != nil {
		return QuotaResult{Message: "Qoder connected. Unable to fetch usage: " + err.Error()}
	}
	if status != http.StatusOK {
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return QuotaResult{Plan: "Qoder", Message: fmt.Sprintf("Qoder token expired or unauthorized (%d).", status)}
		}
		if status == http.StatusNotFound {
			return QuotaResult{Plan: "Qoder", Message: fmt.Sprintf("Qoder quota endpoint not found (%d).", status)}
		}
		return QuotaResult{Plan: "Qoder", Message: fmt.Sprintf("Qoder connected. Usage fetch returned %d.", status)}
	}
	if len(raw) == 0 {
		return QuotaResult{Message: "Qoder connected. Usage response was not JSON."}
	}
	if data == nil {
		if err := json.Unmarshal([]byte(raw), &data); err != nil {
			return QuotaResult{Message: "Qoder connected. Usage response was not JSON."}
		}
	}

	if uid, ok := data["userId"].(string); ok && uid != "" {
		accountID = uid
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
		if total <= 0 {
			total = num(q["cap"])
		}
		used := num(q["used"])
		remaining := num(q["remaining"])
		if total <= 0 && remaining > 0 {
			total = used + remaining
		}
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
		if pct <= 1.0 {
			pct *= 100
		}
		plan = fmt.Sprintf("Qoder (%.0f%% used)", pct)
	}
	if exceeded, _ := data["isQuotaExceeded"].(bool); exceeded {
		plan = "Qoder (Quota Exceeded)"
	}
	return QuotaResult{Plan: plan, Quotas: quotas, AccountID: accountID}
}
