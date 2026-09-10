// codebuddy.go extracts real quota/usage data from Tencent CodeBuddy billing API.
// Ported from 9router open-sse/services/usage/codebuddy-cn.js.

package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	codebuddyCnBillingURL   = "https://copilot.tencent.com/v2/billing/meter/get-user-resource"
	codebuddyIntlBillingURL = "https://www.codebuddy.ai/v2/billing/meter/get-user-resource"
)

type codebuddyBillingResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Response struct {
			Data struct {
				Accounts []codebuddyAccount `json:"Accounts"`
			} `json:"Data"`
		} `json:"Response"`
	} `json:"data"`
}

type codebuddyAccount struct {
	AccountId                  int64   `json:"AccountId"`
	PackageName                string  `json:"PackageName"`
	SubProductName             string  `json:"SubProductName"`
	CapacityUnit               string  `json:"CapacityUnit"`
	CycleStartTime             string  `json:"CycleStartTime"`
	CycleEndTime               string  `json:"CycleEndTime"`
	DeductionEndTime           int64   `json:"DeductionEndTime"`
	CapacitySize               float64 `json:"CapacitySize"`
	CapacityUsed               float64 `json:"CapacityUsed"`
	CapacityRemain             float64 `json:"CapacityRemain"`
	CycleCapacitySize          float64 `json:"CycleCapacitySize"`
	CycleCapacityUsed          float64 `json:"CycleCapacityUsed"`
	CycleCapacityRemain        float64 `json:"CycleCapacityRemain"`
	CapacitySizePrecise        string  `json:"CapacitySizePrecise"`
	CapacityUsedPrecise        string  `json:"CapacityUsedPrecise"`
	CapacityRemainPrecise      string  `json:"CapacityRemainPrecise"`
	CycleCapacitySizePrecise   string  `json:"CycleCapacitySizePrecise"`
	CycleCapacityUsedPrecise   string  `json:"CycleCapacityUsedPrecise"`
	CycleCapacityRemainPrecise string  `json:"CycleCapacityRemainPrecise"`
	Status                     int     `json:"Status"`
}

func parseCodebuddyFloat(precise string, fallback float64) float64 {
	if s := strings.TrimSpace(precise); s != "" {
		if val, err := strconv.ParseFloat(s, 64); err == nil {
			return val
		}
	}
	return fallback
}

func parseCodebuddyDate(s string) (time.Time, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, ""
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, t.UTC().Format(time.RFC3339)
	}
	cst := time.FixedZone("CST", 8*3600)
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, cst); err == nil {
		return t, t.UTC().Format(time.RFC3339)
	}
	return time.Time{}, s
}

func fetchCodebuddy(ctx context.Context, client *http.Client, c QuotaCredentials) QuotaResult {
	token := strings.TrimSpace(c.AccessToken)
	if token == "" {
		token = strings.TrimSpace(c.APIKey)
	}
	if token == "" {
		return QuotaResult{Message: "CodeBuddy quota unavailable: no access token"}
	}

	targetURL := codebuddyCnBillingURL
	userAgent := "CLI/2.108.1 CodeBuddy/2.108.1"
	ideType := "CLI"

	if strings.Contains(c.Provider, "intl") {
		targetURL = codebuddyIntlBillingURL
		userAgent = "IDE/2.108.1 CodeBuddy/2.108.1"
		ideType = "IDE"
	}
	if c.BaseURL != "" {
		targetURL = c.BaseURL
	}

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return QuotaResult{Message: "CodeBuddy quota request failed: " + err.Error()}
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-Product", "SaaS")
	req.Header.Set("X-IDE-Type", ideType)
	req.Header.Set("X-IDE-Name", ideType)
	req.Header.Set("x-requested-with", "XMLHttpRequest")
	req.Header.Set("x-codebuddy-request", "1")

	resp, err := client.Do(req)
	if err != nil {
		return QuotaResult{Message: "CodeBuddy quota fetch error: " + err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return QuotaResult{Message: "CodeBuddy credential invalid or expired."}
	}
	if resp.StatusCode != http.StatusOK {
		return QuotaResult{Message: fmt.Sprintf("CodeBuddy quota API returned HTTP %d", resp.StatusCode)}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return QuotaResult{Message: "Failed to read CodeBuddy quota response: " + err.Error()}
	}

	var billingResp codebuddyBillingResp
	if err := json.Unmarshal(body, &billingResp); err != nil {
		return QuotaResult{Message: "Failed to parse CodeBuddy quota response: " + err.Error()}
	}
	if billingResp.Code != 0 {
		return QuotaResult{Message: "CodeBuddy quota API error: " + billingResp.Msg}
	}

	accounts := billingResp.Data.Response.Data.Accounts
	if len(accounts) == 0 {
		return QuotaResult{Message: "CodeBuddy connected. No credit package found."}
	}

	const refillGapMs = int64(2 * 24 * 60 * 60 * 1000) // 2 days

	var refills []codebuddyAccount
	var bonuses []codebuddyAccount

	for _, acc := range accounts {
		remain := parseCodebuddyFloat(acc.CapacityRemainPrecise, acc.CapacityRemain)
		cycleRemain := parseCodebuddyFloat(acc.CycleCapacityRemainPrecise, acc.CycleCapacityRemain)
		// Skip fully exhausted/expired packages (e.g. Status != 0 with no remaining balance)
		if acc.Status != 0 && remain <= 0 && cycleRemain <= 0 {
			continue
		}

		cycleEnd, _ := parseCodebuddyDate(acc.CycleEndTime)
		var cycleEndMs int64
		if !cycleEnd.IsZero() {
			cycleEndMs = cycleEnd.UnixMilli()
		}

		if cycleEndMs > 0 && acc.DeductionEndTime > 0 && (acc.DeductionEndTime-cycleEndMs) > refillGapMs {
			refills = append(refills, acc)
		} else {
			bonuses = append(bonuses, acc)
		}
	}

	quotas := make(map[string]Quota)
	plan := "CodeBuddy"

	for i, acc := range refills {
		name := "Monthly"
		if i > 0 {
			name = fmt.Sprintf("Monthly %d", i+1)
		}
		total := parseCodebuddyFloat(acc.CycleCapacitySizePrecise, acc.CycleCapacitySize)
		used := parseCodebuddyFloat(acc.CycleCapacityUsedPrecise, acc.CycleCapacityUsed)
		remaining := parseCodebuddyFloat(acc.CycleCapacityRemainPrecise, acc.CycleCapacityRemain)
		if remaining <= 0 && total > used {
			remaining = total - used
		}
		pct := 0.0
		if total > 0 {
			pct = (remaining / total) * 100
		}
		_, resetISO := parseCodebuddyDate(acc.CycleEndTime)
		unit := acc.CapacityUnit
		if unit == "" {
			unit = "credits"
		}
		dispName := acc.PackageName
		if dispName == "" {
			dispName = acc.SubProductName
		}
		if plan == "CodeBuddy" && dispName != "" {
			plan = dispName
		}
		quotas[name] = Quota{
			Used:                used,
			Total:               total,
			Remaining:           remaining,
			RemainingPercentage: pct,
			ResetAt:             resetISO,
			Unit:                unit,
			DisplayName:         dispName,
		}
	}

	for i, acc := range bonuses {
		name := fmt.Sprintf("Bonus Pack %d", i+1)
		total := parseCodebuddyFloat(acc.CapacitySizePrecise, acc.CapacitySize)
		used := parseCodebuddyFloat(acc.CapacityUsedPrecise, acc.CapacityUsed)
		remaining := parseCodebuddyFloat(acc.CapacityRemainPrecise, acc.CapacityRemain)
		if remaining <= 0 && total > used {
			remaining = total - used
		}
		pct := 0.0
		if total > 0 {
			pct = (remaining / total) * 100
		}
		_, resetISO := parseCodebuddyDate(acc.CycleEndTime)
		if resetISO == "" && acc.DeductionEndTime > 0 {
			resetISO = time.UnixMilli(acc.DeductionEndTime).UTC().Format(time.RFC3339)
		}
		unit := acc.CapacityUnit
		if unit == "" {
			unit = "credits"
		}
		dispName := acc.PackageName
		if dispName == "" {
			dispName = acc.SubProductName
		}
		if plan == "CodeBuddy" && dispName != "" {
			plan = dispName
		}
		quotas[name] = Quota{
			Used:                used,
			Total:               total,
			Remaining:           remaining,
			RemainingPercentage: pct,
			ResetAt:             resetISO,
			Unit:                unit,
			DisplayName:         dispName,
		}
	}

	if len(quotas) == 0 {
		return QuotaResult{Plan: plan, Message: "CodeBuddy connected. No active quota available."}
	}

	return QuotaResult{Plan: plan, Quotas: quotas}
}
