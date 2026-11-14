// antigravity.go reads Google Code Assist quota. retrieveUserQuotaSummary is the
// primary source: it reports the grouped pools with upstream-owned bucket ids and
// window tokens. fetchAvailableModels is the fallback and only exposes one quota
// window per model. Ported from 9router and picoclaw.

package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/provider"
)

type AntigravityModelsResponse struct {
	Models map[string]AntigravityModelEntry `json:"models"`
}

// AntigravityModelEntry is one advertised model. Servability is upstream's own
// statement, not a reading of the id: isInternal and the INTERNAL api surface mark
// test endpoints, requiresLeadInGeneration marks the editor's inline-completion
// anchors (they carry quotaInfo but no chat surface), and disabled placeholders
// advertise no quotaInfo at all.
type AntigravityModelEntry struct {
	DisplayName    string `json:"displayName"`
	APIProvider    string `json:"apiProvider"`
	IsInternal     bool   `json:"isInternal"`
	Disabled       bool   `json:"disabled"`
	RequiresLeadIn bool   `json:"requiresLeadInGeneration"`
	QuotaInfo      *struct {
		RemainingFraction *float64 `json:"remainingFraction"`
		ResetTime         string   `json:"resetTime"`
		IsExhausted       bool     `json:"isExhausted"`
	} `json:"quotaInfo"`
}

// AntigravityQuotaSummaryResponse mirrors /v1internal:retrieveUserQuotaSummary:
// upstream-owned groups, each carrying the quota windows it exposes. The bucket id
// and window token are the only stable pool identifiers Google returns, so they
// drive both the map key and the label of every panel row.
type AntigravityQuotaSummaryResponse struct {
	Groups []AntigravityQuotaGroup `json:"groups"`
}

type AntigravityQuotaGroup struct {
	DisplayName string                   `json:"displayName"`
	Description string                   `json:"description"`
	Buckets     []AntigravityQuotaBucket `json:"buckets"`
}

type AntigravityQuotaBucket struct {
	BucketID          string   `json:"bucketId"`
	Window            string   `json:"window"`
	RemainingFraction *float64 `json:"remainingFraction"`
	ResetTime         string   `json:"resetTime"`
	DisplayName       string   `json:"displayName"`
	Description       string   `json:"description"`
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
		// Both quota endpoints sit behind a CDN that caches a POST response for ~20s
		// unless the caller says otherwise: a refresh could otherwise keep showing the
		// pre-reset numbers. Same header the upstream CLI sends.
		req.Header.Set("X-Goog-Cloud-Code-Hybrid-Quota-No-Cache", "true")

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

	// 2. Second attempt: fetchAvailableModels, one row per advertised model window.
	// This endpoint carries no pool identity, so no aggregate is synthesized.
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

// clusterAntigravityQuotas turns the fallback endpoint's per-model windows into
// panel rows. fetchAvailableModels never names the pool a window belongs to, so
// each advertised model gets its own row keyed by that upstream model id.
// Collapsing models into invented aggregates ("gemini", "claude", "gemini4")
// displayed labels upstream never uses and hid which model actually drained.
// Upstream reports a fraction and a reset time and no request count, so no
// absolute figure is published either.
func clusterAntigravityQuotas(parsed AntigravityModelsResponse) map[string]Quota {
	if len(parsed.Models) == 0 {
		return nil
	}

	quotas := make(map[string]Quota, len(parsed.Models))
	for id, m := range parsed.Models {
		// Upstream flags its test endpoints and inline-completion anchors; entries it
		// marks disabled advertise no quotaInfo and drop out on the nil guard.
		if m.QuotaInfo == nil ||
			provider.AntigravityInternalEngine(m.IsInternal, m.APIProvider, m.RequiresLeadIn) {
			continue
		}

		remFrac := 1.0
		if m.QuotaInfo.RemainingFraction != nil {
			remFrac = *m.QuotaInfo.RemainingFraction
		}
		if m.QuotaInfo.IsExhausted {
			remFrac = 0.0
		}

		displayName := strings.TrimSpace(m.DisplayName)
		if displayName == "" {
			displayName = id
		}
		resetAt, resetSoon := antigravityReset(m.QuotaInfo.ResetTime)

		quotas[id] = Quota{
			DisplayName:         displayName,
			RemainingPercentage: remFrac * 100.0,
			ResetAt:             resetAt,
			ResetSoon:           resetSoon,
		}
	}
	return quotas
}

// clusterAntigravitySummary keys each row by the upstream bucket id (live:
// gemini-5h, gemini-weekly, 3p-5h, 3p-weekly, gemini-argon-wtu-weekly) and labels
// it with the upstream group name plus window token. Keyword matching on the group
// name used to rewrite those into "gemini_session"/"claude_weekly" with constant
// labels, which hid upstream copy changes behind strings we had to keep current by
// hand — and the "gemini"/"gemini4" collision forced an Argon special case.
func clusterAntigravitySummary(summary AntigravityQuotaSummaryResponse) map[string]Quota {
	quotas := make(map[string]Quota, len(summary.Groups)*2)
	for _, g := range summary.Groups {
		for _, b := range g.Buckets {
			key := strings.ToLower(strings.TrimSpace(b.BucketID))
			if key == "" {
				continue // nothing stable to key or translate on
			}

			remFrac := 1.0
			if b.RemainingFraction != nil {
				remFrac = *b.RemainingFraction
			}
			resetAt, resetSoon := antigravityReset(b.ResetTime)

			quotas[uniqueQuotaKey(quotas, key)] = Quota{
				DisplayName:         antigravityBucketLabel(g, b),
				RemainingPercentage: remFrac * 100.0,
				ResetAt:             resetAt,
				ResetSoon:           resetSoon,
			}
		}
	}
	return quotas
}

// antigravityBucketLabel picks the most human upstream string available: group and
// window together, then the bucket's own name, then the bucket id plus window. A
// pool this build never saw still reads as upstream text instead of vanishing
// behind a name we failed to predict.
func antigravityBucketLabel(g AntigravityQuotaGroup, b AntigravityQuotaBucket) string {
	group := strings.TrimSpace(g.DisplayName)
	window := strings.TrimSpace(b.Window)
	bucket := strings.TrimSpace(b.BucketID)
	name := strings.TrimSpace(b.DisplayName)
	switch {
	case group != "" && window != "":
		return group + " · " + window
	case group != "":
		return group
	case name != "":
		return name
	case window != "":
		return bucket + " · " + window
	}
	return bucket
}

// uniqueQuotaKey keeps a repeated upstream bucket id from overwriting an earlier
// row: Google reuses window ids across plan tiers on some accounts.
func uniqueQuotaKey(quotas map[string]Quota, key string) string {
	if _, taken := quotas[key]; !taken {
		return key
	}
	for suffix := 2; ; suffix++ {
		candidate := key + "-" + strconv.Itoa(suffix)
		if _, taken := quotas[candidate]; !taken {
			return candidate
		}
	}
}

// antigravityReset normalizes an upstream reset stamp. Google returns either an
// RFC3339 instant or a localized "resets soon" phrase; an instant already in the
// past is reported as imminent rather than as a future deadline, so selection
// ignores it instead of pinning traffic to a stale snapshot.
func antigravityReset(raw string) (resetAt string, resetSoon bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC().Format(time.RFC3339), !t.After(time.Now())
		}
	}
	if raw == "即将重置" || strings.EqualFold(raw, "reset soon") || strings.EqualFold(raw, "resetting soon") {
		return "", true
	}
	return "", false
}
