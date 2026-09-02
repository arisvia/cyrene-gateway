// quota_providers.go holds per-provider quota fetchers ported from 9router
// open-sse/services/usage/ (Phase 31). Endpoints mirror the registry
// transport.usage blocks; parsing matches the JS handlers field-for-field so
// the panel renders identically.

package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// parseResetTime normalizes a provider reset value (unix seconds/ms, numeric
// string, or ISO string) to an RFC3339 string. Port of 9router parseResetTime.
func parseResetTime(v any) string {
	switch val := v.(type) {
	case float64:
		return unixToISO(val)
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return ""
		}
		if n, err := strconv.ParseFloat(s, 64); err == nil {
			return unixToISO(n)
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return ""
}

// unixToISO converts a unix timestamp that may be seconds or milliseconds.
func unixToISO(ms float64) string {
	if ms <= 0 {
		return ""
	}
	if ms < 1e12 {
		ms *= 1000
	}
	return time.UnixMilli(int64(ms)).UTC().Format(time.RFC3339)
}

// num coerces a JSON number/string to float64 (0 fallback). Port of toFiniteNumber.
func num(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil {
			return f
		}
	}
	return 0
}

// getJSON performs a GET with Bearer auth and decodes the JSON body.
func getJSON(ctx context.Context, client *http.Client, url, token string) (int, map[string]any, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
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

// ── DeepSeek ─────────────────────────────────────────────────────────────
// GET https://api.deepseek.com/user/balance — port of getDeepseekUsage.

const deepseekBalanceURL = "https://api.deepseek.com/user/balance"

// endpoint resolves a fetcher's target URL, honoring the test BaseURL override.
// When override is set it replaces the scheme+host of the default URL, keeping
// the path so a mock server can serve the same route.
func endpoint(defaultURL, override string) string {
	if override == "" {
		return defaultURL
	}
	_, after, _ := strings.Cut(defaultURL, "://")
	path := after
	if slash := strings.Index(path, "/"); slash >= 0 {
		return strings.TrimRight(override, "/") + path[slash:]
	}
	return strings.TrimRight(override, "/")
}

func fetchDeepseek(ctx context.Context, client *http.Client, c QuotaCredentials) QuotaResult {
	if strings.TrimSpace(c.APIKey) == "" {
		return QuotaResult{Message: "DeepSeek API key not available. Add a key to view usage."}
	}
	status, data, _, err := getJSON(ctx, client, endpoint(deepseekBalanceURL, c.BaseURL), strings.TrimSpace(c.APIKey))
	if err != nil {
		return QuotaResult{Message: "DeepSeek error: " + err.Error()}
	}
	if status == 401 || status == 403 {
		return QuotaResult{Plan: "DeepSeek", Message: "DeepSeek authentication failed. Check the API key."}
	}
	if status != 200 {
		return QuotaResult{Plan: "DeepSeek", Message: fmt.Sprintf("DeepSeek balance API error (%d)", status)}
	}

	list, _ := data["balance_infos"].([]any)
	if len(list) == 0 {
		return QuotaResult{Plan: "DeepSeek", Message: "DeepSeek connected. No balance data returned."}
	}

	quotas := map[string]Quota{}
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		currency := strings.ToUpper(fmt.Sprint(m["currency"]))
		if currency == "" {
			continue
		}
		total := num(orKey(m, "total_balance", "totalBalance"))
		if total < 0 {
			total = 0
		}
		remainingPct := 0.0
		if total > 0 {
			remainingPct = 100
		}
		quotas["Balance ("+currency+")"] = Quota{
			Used:                0,
			Total:               total,
			RemainingPercentage: remainingPct,
			Unlimited:           total > 0,
		}
	}
	if len(quotas) == 0 {
		return QuotaResult{Plan: "DeepSeek", Message: "DeepSeek connected. No balance data returned."}
	}

	plan := "DeepSeek"
	if avail, ok := data["is_available"].(bool); ok && !avail {
		plan = "DeepSeek (Insufficient Balance)"
	}
	return QuotaResult{Plan: plan, Quotas: quotas}
}

// ── GLM ──────────────────────────────────────────────────────────────────
// Region-aware quota endpoints — port of getGlmUsage (misc.js).

var glmQuotaURLs = map[string]string{
	"glm":    "https://api.z.ai/api/monitor/usage/quota/limit",
	"glm-cn": "https://open.bigmodel.cn/api/monitor/usage/quota/limit",
}

func fetchGLM(ctx context.Context, client *http.Client, c QuotaCredentials) QuotaResult {
	if c.APIKey == "" {
		return QuotaResult{Message: "GLM API key not available."}
	}
	url, ok := glmQuotaURLs[c.Provider]
	if !ok {
		url = glmQuotaURLs["glm"]
	}
	status, data, _, err := getJSON(ctx, client, endpoint(url, c.BaseURL), c.APIKey)
	if err != nil {
		return QuotaResult{Message: "GLM error: " + err.Error()}
	}
	if status == 401 {
		return QuotaResult{Message: "GLM API key invalid or expired."}
	}
	if status != 200 {
		return QuotaResult{Message: fmt.Sprintf("GLM quota API error (%d).", status)}
	}

	inner, _ := data["data"].(map[string]any)
	limits, _ := inner["limits"].([]any)
	quotas := map[string]Quota{}
	for _, l := range limits {
		lm, ok := l.(map[string]any)
		if !ok || fmt.Sprint(lm["type"]) != "TOKENS_LIMIT" {
			continue
		}
		usedPct := num(lm["percentage"])
		remaining := 100 - usedPct
		if remaining < 0 {
			remaining = 0
		}
		quotas["session"] = Quota{
			Used:                usedPct,
			Total:               100,
			Remaining:           remaining,
			RemainingPercentage: remaining,
			ResetAt:             parseResetTime(lm["nextResetTime"]),
		}
	}

	plan := "Unknown"
	if level, ok := inner["level"].(string); ok && level != "" {
		plan = strings.ToUpper(level[:1]) + strings.ToLower(level[1:])
	}
	return QuotaResult{Plan: plan, Quotas: quotas}
}

// ── MiniMax ──────────────────────────────────────────────────────────────
// Try endpoints in order with fallback — port of getMiniMaxUsage.

var minimaxUsageURLs = map[string][]string{
	"minimax": {
		"https://www.minimax.io/v1/token_plan/remains",
		"https://api.minimax.io/v1/api/openplatform/coding_plan/remains",
	},
	"minimax-cn": {
		"https://www.minimaxi.com/v1/api/openplatform/coding_plan/remains",
		"https://api.minimaxi.com/v1/api/openplatform/coding_plan/remains",
	},
}

var minimaxAuthLike = regexp.MustCompile(`(?i)token plan|coding plan|invalid api key|invalid key|unauthorized|inactive`)

func fetchMiniMax(ctx context.Context, client *http.Client, c QuotaCredentials) QuotaResult {
	if c.APIKey == "" {
		return QuotaResult{Message: "MiniMax API key not available."}
	}
	urls := minimaxUsageURLs[c.Provider]
	if len(urls) == 0 {
		urls = minimaxUsageURLs["minimax"]
	}
	lastErr := ""
	for i, url := range urls {
		canFallback := i < len(urls)-1
		status, payload, raw, err := getJSON(ctx, client, endpoint(url, c.BaseURL), c.APIKey)
		if err != nil {
			lastErr = err.Error()
			if !canFallback {
				break
			}
			continue
		}

		baseResp, _ := orMap(payload, "base_resp", "baseResp")
		statusCode := num(orKey(baseResp, "status_code", "statusCode"))
		statusMsg := strings.TrimSpace(fmt.Sprint(orKey(baseResp, "status_msg", "statusMsg")))
		combined := strings.TrimSpace(statusMsg + " " + raw)

		if status == 401 || status == 403 || statusCode == 1004 || minimaxAuthLike.MatchString(combined) {
			return QuotaResult{Message: "MiniMax API key invalid or inactive. Use an active Token/Coding Plan key."}
		}
		if status != 200 {
			lastErr = fmt.Sprintf("MiniMax usage endpoint error (%d)", status)
			if (status == 404 || status == 405 || status >= 500) && canFallback {
				continue
			}
			return QuotaResult{Message: "MiniMax connected. " + lastErr}
		}
		if statusCode != 0 {
			msg := statusMsg
			if msg == "" {
				msg = "Upstream quota API error"
			}
			return QuotaResult{Message: "MiniMax connected. " + msg}
		}

		modelRemains, _ := orSlice(payload, "model_remains", "modelRemains")
		countMeansRemaining := strings.Contains(url, "/coding_plan/remains")
		capturedAt := time.Now()
		quotas := map[string]Quota{}

		for _, m := range modelRemains {
			model, ok := m.(map[string]any)
			if !ok || !minimaxHasQuota(model) {
				continue
			}
			name := minimaxDisplayName(model)
			minimaxAddQuota(quotas, name+" (5h)", model, capturedAt, countMeansRemaining, false)
			minimaxAddQuota(quotas, name+" (7d)", model, capturedAt, countMeansRemaining, true)
		}
		if len(quotas) == 0 {
			return QuotaResult{Message: "MiniMax connected. No quota data was returned."}
		}
		return QuotaResult{Quotas: quotas}
	}
	if lastErr == "" {
		lastErr = "Unable to fetch usage."
	}
	return QuotaResult{Message: "MiniMax connected. Unable to fetch usage: " + lastErr}
}

func minimaxHasQuota(m map[string]any) bool {
	if num(orKey(m, "current_interval_total_count", "currentIntervalTotalCount")) > 0 ||
		num(orKey(m, "current_weekly_total_count", "currentWeeklyTotalCount")) > 0 {
		return true
	}
	if _, ok := orKeyOK(m, "current_interval_remaining_percent", "currentIntervalRemainingPercent"); ok {
		return true
	}
	_, ok := orKeyOK(m, "current_weekly_remaining_percent", "currentWeeklyRemainingPercent")
	return ok
}

func minimaxDisplayName(m map[string]any) string {
	raw := strings.TrimSpace(fmt.Sprint(orKey(m, "model_name", "modelName")))
	if raw == "" || raw == "<nil>" {
		return "MiniMax"
	}
	if raw == "MiniMax-M*" || raw == "general" {
		return "M-series"
	}
	return raw
}

func minimaxAddQuota(quotas map[string]Quota, key string, m map[string]any, capturedAt time.Time, countMeansRemaining, weekly bool) {
	var totalKey, countKey, pctKey, remainsKey, endKey [2]string
	if weekly {
		totalKey = [2]string{"current_weekly_total_count", "currentWeeklyTotalCount"}
		countKey = [2]string{"current_weekly_usage_count", "currentWeeklyUsageCount"}
		pctKey = [2]string{"current_weekly_remaining_percent", "currentWeeklyRemainingPercent"}
		remainsKey = [2]string{"weekly_remains_time", "weeklyRemainsTime"}
		endKey = [2]string{"weekly_end_time", "weeklyEndTime"}
	} else {
		totalKey = [2]string{"current_interval_total_count", "currentIntervalTotalCount"}
		countKey = [2]string{"current_interval_usage_count", "currentIntervalUsageCount"}
		pctKey = [2]string{"current_interval_remaining_percent", "currentIntervalRemainingPercent"}
		remainsKey = [2]string{"remains_time", "remainsTime"}
		endKey = [2]string{"end_time", "endTime"}
	}

	total := num(m[totalKey[0]])
	if total == 0 {
		total = num(m[totalKey[1]])
	}
	providedPct, hasPct := minimaxPct(m, pctKey)
	if total <= 0 && !hasPct {
		return
	}

	count := num(m[countKey[0]])
	if count == 0 {
		count = num(m[countKey[1]])
	}
	if count < 0 {
		count = 0
	}

	effectiveTotal := total
	effectiveCount := count
	if total <= 0 {
		// M-series bucket: percent-only. Normalize to total=100.
		effectiveTotal = 100
		if countMeansRemaining {
			effectiveCount = float64(int(effectiveTotal*(providedPct/100) + 0.5))
		} else {
			effectiveCount = float64(int(effectiveTotal*(1-providedPct/100) + 0.5))
		}
	}

	var used float64
	if countMeansRemaining {
		used = effectiveTotal - effectiveCount
		if used < 0 {
			used = 0
		}
	} else {
		used = effectiveCount
		if used > effectiveTotal {
			used = effectiveTotal
		}
	}
	remaining := effectiveTotal - used
	if remaining < 0 {
		remaining = 0
	}
	remainingPct := providedPct
	if !hasPct {
		if effectiveTotal > 0 {
			remainingPct = remaining / effectiveTotal * 100
		} else {
			remainingPct = 0
		}
	}
	if remainingPct < 0 {
		remainingPct = 0
	}
	if remainingPct > 100 {
		remainingPct = 100
	}

	resetAt := minimaxResetAt(m, capturedAt, remainsKey, endKey)
	quotas[key] = Quota{
		Used:                used,
		Total:               effectiveTotal,
		Remaining:           remaining,
		RemainingPercentage: remainingPct,
		ResetAt:             resetAt,
	}
}

func minimaxPct(m map[string]any, key [2]string) (float64, bool) {
	v, ok := orKeyOK(m, key[0], key[1])
	if !ok || v == nil {
		return 0, false
	}
	f := num(v)
	if f < 0 {
		f = 0
	}
	if f > 100 {
		f = 100
	}
	return f, true
}

func minimaxResetAt(m map[string]any, capturedAt time.Time, remainsKey, endKey [2]string) string {
	remainsMs := num(m[remainsKey[0]])
	if remainsMs == 0 {
		remainsMs = num(m[remainsKey[1]])
	}
	if remainsMs > 0 {
		return capturedAt.Add(time.Duration(remainsMs) * time.Millisecond).UTC().Format(time.RFC3339)
	}
	return parseResetTime(orKey(m, endKey[0], endKey[1]))
}

// ── small map helpers ────────────────────────────────────────────────────

func orKey(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

func orKeyOK(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return nil, false
}

func orMap(m map[string]any, keys ...string) (map[string]any, bool) {
	if v, ok := orKeyOK(m, keys...); ok {
		if mm, ok := v.(map[string]any); ok {
			return mm, true
		}
	}
	return map[string]any{}, false
}

func orSlice(m map[string]any, keys ...string) ([]any, bool) {
	if v, ok := orKeyOK(m, keys...); ok {
		if s, ok := v.([]any); ok {
			return s, true
		}
	}
	return nil, false
}
