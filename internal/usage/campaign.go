// campaign.go adapts the upstream per-account promotional activities that
// hand out free Credits: Qoder's "Claim 100 Credits Every Day" campaign API
// (GET /sash/api/v1/me/campaigns + POST .../claim) and CodeBuddy/WorkBuddy's
// "Buddy gas station" daily check-in (POST /v2/billing/meter/checkin-activity-status
// + daily-checkin). Both upstreams are idempotent per reward window, so
// re-claiming is safe; we only relay the same requests the official clients
// make with the account's own stored credential.

package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// Campaign status values surfaced to the panel.
const (
	CampaignStatusClaimable = "claimable"
	CampaignStatusClaimed   = "claimed"
	CampaignStatusInactive  = "inactive"
)

// Claim outcome values surfaced by the claim endpoint.
const (
	ClaimOutcomeClaimed = "claimed"
	ClaimOutcomeAlready = "already"
	ClaimOutcomeNone    = "none"
	ClaimOutcomeError   = "error"
)

// Campaign is one grant activity attached to an upstream account.
type Campaign struct {
	ID     string  `json:"id"`
	Status string  `json:"status"`
	Amount float64 `json:"amount,omitempty"`
	Unit   string  `json:"unit,omitempty"`
	EndsAt string  `json:"endsAt,omitempty"` // RFC3339 when the reward window closes
}

// ClaimSummary is the result of a one-click claim-all attempt.
type ClaimSummary struct {
	Supported bool    `json:"supported"`
	Outcome   string  `json:"outcome"`
	Amount    float64 `json:"amount,omitempty"`
	Count     int     `json:"count,omitempty"`
	Message   string  `json:"message,omitempty"`
}

// campaignKind resolves the upstream activity adapter for a provider via the
// registry. Empty means the provider has no known claimable activity.
func campaignKind(providerID string) string {
	info, ok := provider.GetProvider(providerID)
	if !ok {
		return ""
	}
	return info.CampaignKind
}

// CampaignSupported reports whether a provider has a claimable-activity adapter.
func CampaignSupported(providerID string) bool {
	return campaignKind(providerID) != ""
}

// ClaimCampaigns dispatches a check-then-claim round for every claimable
// activity on the connection's account.
func ClaimCampaigns(ctx context.Context, client *http.Client, c QuotaCredentials) ClaimSummary {
	switch campaignKind(c.Provider) {
	case "qoder":
		return claimQoderCampaigns(ctx, quotaHTTPClient(client), c)
	case "codebuddy":
		return claimCodebuddyCheckin(ctx, quotaHTTPClient(client), c)
	}
	return ClaimSummary{Supported: false, Outcome: ClaimOutcomeError, Message: "No campaign API for " + c.Provider}
}

// ── shared POST helper (mirrors getJSON) ──────────────────────────────────

func postJSON(ctx context.Context, client *http.Client, url string, headers map[string]string, body []byte) (int, map[string]any, string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
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

func strAny(v any) string {
	s, _ := v.(string)
	return s
}

func boolAny(v any) bool {
	b, _ := v.(bool)
	return b
}

// ── Qoder: /sash/api/v1/me/campaigns ───────────────────────────────────────

// qoderCampaignBase returns the openapi host for the region; the campaigns
// endpoint lives on the same host as the device-token/quota APIs.
func qoderCampaignBase(providerID string) string {
	if strings.Contains(providerID, "cn") {
		return "https://openapi.qoder.com.cn"
	}
	return "https://openapi.qoder.sh"
}

func qoderCampaignURL(c QuotaCredentials, campaignID string) string {
	defaultURL := qoderCampaignBase(c.Provider) + "/sash/api/v1/me/campaigns"
	if campaignID != "" {
		defaultURL += "/" + campaignID + "/claim"
	}
	return endpoint(defaultURL, c.BaseURL)
}

// qoderCampaignHeaders replicates the claim-capable client identity: the
// campaigns list is served empty without Cosy-ClientType=10, and the daily
// grant is gated on the account's registered device identity. The machine
// fields mirror the values the gateway already signs inference calls with,
// so the claim looks like the same install rather than a second device.
func qoderCampaignHeaders(token, machineID string) map[string]string {
	h := bearer(token)
	h["User-Agent"] = provider.QoderCLIUserAgent
	h[provider.QoderCosyVersionHeader] = provider.QoderIDEVersion
	h["Cosy-ClientType"] = "10"
	if machineID != "" {
		h["Cosy-MachineId"] = machineID
		h["Cosy-MachineToken"] = machineID
		h["Cosy-MachineType"] = "5"
		h["Cosy-MachineOS"] = "x86_64_windows"
	}
	return h
}

// qoderAccountToken resolves PAT connections to the job token the openapi
// hosts accept (same exchange the quota fetcher performs).
func qoderAccountToken(ctx context.Context, client *http.Client, c QuotaCredentials) (string, string, error) {
	token := strings.TrimSpace(c.AccessToken)
	if token == "" {
		token = strings.TrimSpace(c.APIKey)
	}
	if token == "" {
		return "", "", fmt.Errorf("no qoder access token on connection")
	}
	if provider.IsQoderPAT(token) {
		resolved, err := provider.ResolveQoderCredentialFor(c.Provider, token, "", client)
		if err != nil {
			return "", "", err
		}
		return resolved.AccessToken, resolved.UserID, nil
	}
	return token, "", nil
}

// qoderCampaignState lists the benefit campaigns on the account. The second
// return value is the human-readable upstream failure, "" on success.
func qoderCampaignState(ctx context.Context, client *http.Client, c QuotaCredentials) ([]Campaign, string) {
	token, _, err := qoderAccountToken(ctx, client, c)
	if err != nil {
		return nil, "Qoder campaign request failed: " + err.Error()
	}
	status, data, _, err := getJSON(ctx, client, qoderCampaignURL(c, ""), qoderCampaignHeaders(token, c.MachineID))
	if err != nil {
		return nil, "Qoder campaign fetch error: " + err.Error()
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, fmt.Sprintf("Qoder token expired or unauthorized (%d).", status)
	}
	if status != http.StatusOK {
		return nil, fmt.Sprintf("Qoder campaign API returned HTTP %d", status)
	}
	list, _ := orSlice(data, "campaigns")
	var out []Campaign
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok || strAny(m["actionType"]) != "CLAIM_BENEFIT" {
			continue
		}
		id := strAny(m["campaignId"])
		if id == "" {
			continue
		}
		cpStatus := CampaignStatusInactive
		switch strAny(m["claimStatus"]) {
		case "CLAIMABLE":
			cpStatus = CampaignStatusClaimable
		case "CLAIMED":
			cpStatus = CampaignStatusClaimed
		}
		benefit, _ := orMap(m, "benefit")
		var amount float64
		if benefit != nil {
			amount = num(benefit["amount"])
		}
		unit := ""
		if benefit != nil {
			unit = strAny(benefit["unit"])
		}
		if unit == "" {
			unit = "credits"
		}
		out = append(out, Campaign{
			ID:     id,
			Status: cpStatus,
			Amount: amount,
			Unit:   unit,
			EndsAt: parseResetTime(m["endAt"]),
		})
	}
	return out, ""
}

func claimQoderCampaigns(ctx context.Context, client *http.Client, c QuotaCredentials) ClaimSummary {
	campaigns, fail := qoderCampaignState(ctx, client, c)
	if fail != "" {
		return ClaimSummary{Supported: true, Outcome: ClaimOutcomeError, Message: fail}
	}
	var claimable []Campaign
	var already bool
	for _, cp := range campaigns {
		if cp.Status == CampaignStatusClaimable {
			claimable = append(claimable, cp)
		} else if cp.Status == CampaignStatusClaimed {
			already = true
		}
	}
	if len(claimable) == 0 {
		if already {
			return ClaimSummary{Supported: true, Outcome: ClaimOutcomeAlready}
		}
		return ClaimSummary{Supported: true, Outcome: ClaimOutcomeNone}
	}

	token, _, err := qoderAccountToken(ctx, client, c)
	if err != nil {
		return ClaimSummary{Supported: true, Outcome: ClaimOutcomeError, Message: err.Error()}
	}
	headers := qoderCampaignHeaders(token, c.MachineID)

	summary := ClaimSummary{Supported: true}
	var errs []string
	for _, cp := range claimable {
		status, data, _, err := postJSON(ctx, client, qoderCampaignURL(c, cp.ID), headers, []byte("{}"))
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		inner, _ := orMap(data, "data")
		if inner == nil {
			inner = data
		}
		// Upstream answers {status:"CLAIMED", replayed:bool}; replayed=true means
		// an earlier claim already landed — treat it as success, not a miss.
		if status == http.StatusOK && strAny(inner["status"]) == "CLAIMED" {
			summary.Count++
			if cp.Amount > 0 {
				summary.Amount += cp.Amount
			} else if benefit, ok := orMap(inner, "benefit"); ok {
				summary.Amount += num(benefit["amount"])
			}
			continue
		}
		msg := strAny(inner["message"])
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", status)
		}
		errs = append(errs, msg)
	}
	switch {
	case summary.Count > 0:
		summary.Outcome = ClaimOutcomeClaimed
		if len(errs) > 0 {
			summary.Message = strings.Join(errs, "; ")
		}
	case len(errs) > 0:
		summary.Outcome = ClaimOutcomeError
		summary.Message = strings.Join(errs, "; ")
	default:
		summary.Outcome = ClaimOutcomeNone
	}
	return summary
}

// ── CodeBuddy: /v2/billing/meter daily check-in ────────────────────────────

const (
	codebuddyCheckinStatusPath = "/v2/billing/meter/checkin-activity-status"
	codebuddyCheckinClaimPath  = "/v2/billing/meter/daily-checkin"
)

func codebuddyCheckinURL(c QuotaCredentials, path string) string {
	host := "https://copilot.tencent.com"
	if c.Provider == "codebuddy" {
		host = "https://www.codebuddy.ai"
	}
	return endpoint(host+path, c.BaseURL)
}

// codebuddyCheckinHeaders mirrors the billing quota call identity; the meter
// namespace rejects requests without the CodeBuddy plugin markers.
func codebuddyCheckinHeaders(token, providerID string) map[string]string {
	h := bearer(token)
	userAgent := provider.CodebuddyCLIUA
	ideType := "CLI"
	if providerID == "codebuddy" {
		userAgent = provider.CodebuddyIDEUA
		ideType = "IDE"
	}
	h["Content-Type"] = "application/json"
	h["User-Agent"] = userAgent
	h["X-Product"] = "SaaS"
	h["X-IDE-Type"] = ideType
	h["X-IDE-Name"] = ideType
	h["x-requested-with"] = "XMLHttpRequest"
	h["x-codebuddy-request"] = "1"
	return h
}

// codebuddyCheckinState reads the daily-checkin activity. The second return
// value carries a human-readable upstream failure, "" on success.
func codebuddyCheckinState(ctx context.Context, client *http.Client, c QuotaCredentials) (Campaign, bool, string) {
	token := strings.TrimSpace(c.AccessToken)
	if token == "" {
		token = strings.TrimSpace(c.APIKey)
	}
	if token == "" {
		return Campaign{}, false, "CodeBuddy check-in unavailable: no access token"
	}
	status, data, _, err := postJSON(ctx, client, codebuddyCheckinURL(c, codebuddyCheckinStatusPath), codebuddyCheckinHeaders(token, c.Provider), []byte("{}"))
	if err != nil {
		return Campaign{}, false, "CodeBuddy check-in status error: " + err.Error()
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return Campaign{}, false, "CodeBuddy credential invalid or expired."
	}
	if status != http.StatusOK {
		return Campaign{}, false, fmt.Sprintf("CodeBuddy check-in API returned HTTP %d", status)
	}
	if code := num(data["code"]); code != 0 {
		msg := strAny(data["msg"])
		if msg == "" {
			msg = fmt.Sprintf("code %v", data["code"])
		}
		return Campaign{}, false, msg
	}
	inner, _ := orMap(data, "data")
	if inner == nil {
		return Campaign{}, false, "CodeBuddy check-in API returned no data"
	}
	if !boolAny(inner["active"]) {
		return Campaign{}, false, ""
	}
	amount := num(inner["today_credit"])
	if amount == 0 {
		amount = num(inner["daily_credit"])
	}
	endsAt := parseResetTime(inner["end_time"])
	if endsAt == "" {
		if _, iso := parseCodebuddyDate(strAny(inner["end_time"])); iso != "" {
			endsAt = iso
		}
	}
	checkedIn := boolAny(inner["today_checked_in"])
	cpStatus := CampaignStatusClaimable
	if checkedIn {
		cpStatus = CampaignStatusClaimed
	}
	return Campaign{
		ID:     "daily-checkin",
		Status: cpStatus,
		Amount: amount,
		Unit:   "credits",
		EndsAt: endsAt,
	}, checkedIn, ""
}

func claimCodebuddyCheckin(ctx context.Context, client *http.Client, c QuotaCredentials) ClaimSummary {
	campaign, checkedIn, fail := codebuddyCheckinState(ctx, client, c)
	if fail != "" {
		return ClaimSummary{Supported: true, Outcome: ClaimOutcomeError, Message: fail}
	}
	if campaign.ID == "" {
		return ClaimSummary{Supported: true, Outcome: ClaimOutcomeNone}
	}
	if checkedIn {
		return ClaimSummary{Supported: true, Outcome: ClaimOutcomeAlready}
	}

	token := strings.TrimSpace(c.AccessToken)
	if token == "" {
		token = strings.TrimSpace(c.APIKey)
	}
	status, data, _, err := postJSON(ctx, client, codebuddyCheckinURL(c, codebuddyCheckinClaimPath), codebuddyCheckinHeaders(token, c.Provider), []byte("{}"))
	if err != nil {
		return ClaimSummary{Supported: true, Outcome: ClaimOutcomeError, Message: "CodeBuddy check-in error: " + err.Error()}
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return ClaimSummary{Supported: true, Outcome: ClaimOutcomeError, Message: "CodeBuddy credential invalid or expired."}
	}
	inner, _ := orMap(data, "data")
	msg := strAny(data["msg"])
	if status == http.StatusOK && num(data["code"]) == 0 {
		summary := ClaimSummary{Supported: true, Outcome: ClaimOutcomeClaimed, Count: 1, Amount: campaign.Amount}
		if inner != nil {
			if amt := num(orKey(inner, "credit", "credits", "today_credit")); amt > 0 {
				summary.Amount = amt
			}
		}
		return summary
	}
	// The upstream has no "already" status code of its own: a repeated claim
	// comes back as a business error saying today is already checked in.
	// Race-free double clicks land here, so fold it into the already outcome.
	if strings.Contains(msg, "已签到") || strings.Contains(msg, "已领取") || strings.Contains(strings.ToLower(msg), "already") {
		return ClaimSummary{Supported: true, Outcome: ClaimOutcomeAlready}
	}
	if msg == "" {
		msg = fmt.Sprintf("CodeBuddy check-in returned HTTP %d", status)
	}
	return ClaimSummary{Supported: true, Outcome: ClaimOutcomeError, Message: msg}
}
