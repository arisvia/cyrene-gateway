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
		_, hasUsed := q["used"]
		remaining := num(q["remaining"])
		_, hasRemaining := q["remaining"]
		if !hasRemaining && hasUsed && total > 0 {
			remaining = total - used
		}
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
		// Exact balances take precedence over ambiguous fractional percentages.
		if total > 0 && (hasRemaining || hasUsed) {
			remainingPct = remaining / total * 100
		} else if pct := num(q["percentage"]); pct > 0 {
			remainingPct = 100 - pct
			if pct <= 1.0 {
				remainingPct = (1.0 - pct) * 100
			}
		}
		if remainingPct < 0 {
			remainingPct = 0
		}
		if remainingPct > 100 {
			remainingPct = 100
		}

		dispName, dispNameI18n := extractQoderDisplayName(q, defaultDisplayName)
		quotas[name] = Quota{
			Used:                used,
			Total:               total,
			Remaining:           remaining,
			RemainingPercentage: remainingPct,
			ResetAt:             parseBucketReset(q, resetAt),
			Unit:                unit,
			DisplayName:         dispName,
			DisplayNameI18n:     dispNameI18n,
		}
	}

	// 1. Process known standard buckets
	userQuota, _ := data["userQuota"].(map[string]any)
	orgQuota, _ := data["orgResourcePackage"].(map[string]any)
	addOnQuota, _ := data["addOnQuota"].(map[string]any)

	extractQuotaBucket("user", userQuota, "")
	extractQuotaBucket("organization", orgQuota, strings.TrimSpace(c.Organization))
	extractQuotaBucket("addOn", addOnQuota, "")

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
	// Dedicated/promotional credits do not establish general quota availability.
	hasAvailableQuota := quotas["user"].Remaining > 0 || quotas["addOn"].Remaining > 0
	if available, ok := orgQuota["available"].(bool); !ok || available {
		hasAvailableQuota = hasAvailableQuota || quotas["organization"].Remaining > 0
	}
	if exceeded, _ := data["isQuotaExceeded"].(bool); exceeded && !hasAvailableQuota {
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
func extractQoderDisplayName(q map[string]any, defaultDisplayName string) (string, map[string]string) {
	labels, _ := orSlice(q, "displayLabels", "display_labels")

	var labelValue string
	var translations map[string]string
	// Select one label: title, then name, then description. Keep its translations
	// together so the UI can switch locales without mixing titles and descriptions.
labelSearch:
	for _, dimension := range []string{"title", "name", "description"} {
		for _, item := range labels {
			label, ok := item.(map[string]any)
			if !ok || label["dimension"] != dimension {
				continue
			}
			i18n, _ := orMap(label, "valueI18n", "value_i18n")
			for locale, value := range i18n {
				if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
					if translations == nil {
						translations = make(map[string]string)
					}
					translations[locale] = strings.TrimSpace(text)
				}
			}
			labelValue, _ = label["value"].(string)
			if strings.TrimSpace(labelValue) != "" || len(translations) > 0 {
				break labelSearch
			}
		}
	}

	// Keep a locale-neutral fallback, excluding internal generated identifiers.
	for _, value := range []any{labelValue, q["title"], q["displayName"], q["packageName"], q["name"]} {
		name, _ := value.(string)
		name = strings.TrimSpace(name)
		if name != "" && !strings.HasPrefix(name, "act-") && !strings.HasPrefix(name, "growth-") && !strings.HasPrefix(name, "pkg-") {
			return name, translations
		}
	}
	return defaultDisplayName, translations
}
