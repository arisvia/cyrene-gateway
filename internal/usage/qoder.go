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

const (
	qoderQuotaUsageURL   = "https://openapi.qoder.sh/api/v2/quota/usage"
	qoderCnQuotaUsageURL = "https://openapi.qoder.com.cn/api/v2/quota/usage"
)

func qoderQuotaURL(providerID, baseURL string) string {
	defaultURL := qoderQuotaUsageURL
	if strings.Contains(providerID, "cn") {
		defaultURL = qoderCnQuotaUsageURL
	}
	if baseURL != "" {
		lower := strings.ToLower(baseURL)
		if strings.Contains(lower, "127.0.0.1") || strings.Contains(lower, "localhost") || strings.Contains(lower, "openapi.qoder") {
			return endpoint(defaultURL, baseURL)
		}
	}
	return defaultURL
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
		resolved, err := provider.ResolveQoderCredentialFor(c.Provider, token, "", client)
		if err != nil {
			return QuotaResult{
				Plan:    "Qoder",
				Message: "Qoder PAT exchange failed (token expired or unauthorized): " + err.Error(),
			}
		}
		token = resolved.AccessToken
		accountID = resolved.UserID
	}

	url := qoderQuotaURL(c.Provider, c.BaseURL)
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

	resetAt := parseQoderReset(num(data["expiresAt"]))

	quotas := map[string]Quota{}
	extractQuotaBucket := func(name string, q map[string]any, defaultDisplayName string) {
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
		if pct := num(q["percentage"]); pct > 0 {
			remainingPct = 100 - pct
			if pct <= 1.0 {
				remainingPct = (1.0 - pct) * 100
			}
		} else if total > 0 {
			remainingPct = remaining / total * 100
		}
		if remainingPct < 0 {
			remainingPct = 0
		}
		if remainingPct > 100 {
			remainingPct = 100
		}

		dispName := extractQoderDisplayName(q, defaultDisplayName)
		quotas[name] = Quota{
			Used:                used,
			Total:               total,
			Remaining:           remaining,
			RemainingPercentage: remainingPct,
			ResetAt:             parseBucketReset(q, resetAt),
			Unit:                unit,
			DisplayName:         dispName,
		}
	}

	// 1. Process known standard buckets
	userQuota, _ := data["userQuota"].(map[string]any)
	orgQuota, _ := data["orgResourcePackage"].(map[string]any)
	addOnQuota, _ := data["addOnQuota"].(map[string]any)

	extractQuotaBucket("user", userQuota, "")
	orgDefaultName := "Organization Credits"
	if strings.TrimSpace(c.Organization) != "" {
		orgDefaultName = strings.TrimSpace(c.Organization)
	}
	extractQuotaBucket("organization", orgQuota, orgDefaultName)
	extractQuotaBucket("addOn", addOnQuota, "Add-on Credits")

	// 2. Dynamically unpack any other quota or package structures in response
	for k, v := range data {
		if k == "userQuota" || k == "orgResourcePackage" || k == "addOnQuota" {
			continue
		}
		lowerK := strings.ToLower(k)
		if m, ok := v.(map[string]any); ok {
			if isQuotaMetricsMap(m) || strings.HasSuffix(lowerK, "quota") || strings.HasSuffix(lowerK, "package") {
				extractQuotaBucket(k, m, formatBucketTitle(k))
			}
		} else if list, ok := v.([]any); ok && (strings.Contains(lowerK, "quota") || strings.Contains(lowerK, "package")) {
			for i, item := range list {
				if itemMap, ok := item.(map[string]any); ok && isQuotaMetricsMap(itemMap) {
					subKey := fmt.Sprintf("%s_%d", k, i+1)
					extractQuotaBucket(subKey, itemMap, formatBucketTitle(subKey))
				}
			}
		}
	}

	// 3. Fallback: If all extracted buckets are empty but userQuota was returned,
	// retain userQuota with 0 balance so the UI reflects 0 credits rather than an error.
	if len(quotas) == 0 && userQuota != nil {
		unit, _ := userQuota["unit"].(string)
		if unit == "" {
			unit = "credits"
		}
		quotas["user"] = Quota{
			Used:                0,
			Total:               0,
			Remaining:           0,
			RemainingPercentage: 0,
			ResetAt:             resetAt,
			Unit:                unit,
		}
	}

	if len(quotas) == 0 {
		return QuotaResult{Message: "Qoder connected. No quota data was returned."}
	}

	plan := "Qoder"
	if ut, ok := data["userType"].(string); ok && strings.TrimSpace(ut) != "" {
		plan = fmt.Sprintf("Qoder (%s)", formatBucketTitle(ut))
	}
	if pct := num(data["totalUsagePercentage"]); pct > 0 {
		if pct <= 1.0 {
			pct *= 100
		}
		plan = fmt.Sprintf("Qoder (%.0f%% used)", pct)
	}
	totalRemaining := 0.0
	for _, q := range quotas {
		totalRemaining += q.Remaining
	}
	if exceeded, _ := data["isQuotaExceeded"].(bool); exceeded && totalRemaining <= 0 {
		plan = "Qoder (Quota Exceeded)"
	}
	return QuotaResult{Plan: plan, Quotas: quotas, AccountID: accountID}
}

func parseQoderReset(expMs float64) string {
	if expMs > 0 && expMs < 200000000000000 {
		return unixToISO(expMs)
	}
	return ""
}

func parseBucketReset(q map[string]any, fallback string) string {
	if exp := num(q["expiresAt"]); exp > 0 {
		return parseQoderReset(exp)
	}
	if exp := num(q["expireTime"]); exp > 0 {
		return parseQoderReset(exp)
	}
	if s, ok := q["resetAt"].(string); ok && s != "" {
		return parseResetTime(s)
	}
	if s, ok := q["validUntil"].(string); ok && s != "" {
		return parseResetTime(s)
	}
	return fallback
}

func isQuotaMetricsMap(m map[string]any) bool {
	if m == nil {
		return false
	}
	_, hasTotal := m["total"]
	_, hasCap := m["cap"]
	_, hasRemaining := m["remaining"]
	_, hasUsed := m["used"]
	return hasTotal || hasCap || hasRemaining || hasUsed
}

func formatBucketTitle(s string) string {
	var words []string
	var cur strings.Builder
	for i, r := range s {
		if r == '_' || r == '-' {
			if cur.Len() > 0 {
				words = append(words, cur.String())
				cur.Reset()
			}
			continue
		}
		if i > 0 && r >= 'A' && r <= 'Z' && s[i-1] >= 'a' && s[i-1] <= 'z' {
			if cur.Len() > 0 {
				words = append(words, cur.String())
				cur.Reset()
			}
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
	}
	return strings.Join(words, " ")
}
func extractQoderDisplayName(q map[string]any, defaultDisplayName string) string {
	if q == nil {
		return defaultDisplayName
	}
	// 1. Prioritize displayLabels / display_labels
	var labels []any
	if l, ok := q["displayLabels"].([]any); ok {
		labels = l
	} else if l, ok := q["display_labels"].([]any); ok {
		labels = l
	}
	for _, item := range labels {
		lm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		dim, _ := lm["dimension"].(string)
		if dim == "title" || dim == "name" {
			if i18n, ok := lm["valueI18n"].(map[string]any); ok {
				if s, ok := i18n["zh-CN"].(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
				if s, ok := i18n["en-US"].(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
			if i18n, ok := lm["value_i18n"].(map[string]any); ok {
				if s, ok := i18n["zh-CN"].(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
				if s, ok := i18n["en-US"].(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
			if v, ok := lm["value"].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	for _, item := range labels {
		lm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		dim, _ := lm["dimension"].(string)
		if dim == "description" {
			if i18n, ok := lm["valueI18n"].(map[string]any); ok {
				if s, ok := i18n["zh-CN"].(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
			if v, ok := lm["value"].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}

	// 2. Check title / displayName / packageName
	if t, ok := q["title"].(string); ok && strings.TrimSpace(t) != "" {
		return strings.TrimSpace(t)
	}
	if dn, ok := q["displayName"].(string); ok && strings.TrimSpace(dn) != "" {
		return strings.TrimSpace(dn)
	}
	if pn, ok := q["packageName"].(string); ok && strings.TrimSpace(pn) != "" {
		return strings.TrimSpace(pn)
	}

	// 3. Check name (filter out internal generated IDs like act-*, growth-*, pkg-*)
	if n, ok := q["name"].(string); ok && strings.TrimSpace(n) != "" {
		trimmed := strings.TrimSpace(n)
		if !strings.HasPrefix(trimmed, "act-") && !strings.HasPrefix(trimmed, "growth-") && !strings.HasPrefix(trimmed, "pkg-") {
			return trimmed
		}
	}

	return defaultDisplayName
}
