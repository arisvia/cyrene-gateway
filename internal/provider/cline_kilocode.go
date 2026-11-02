package provider

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Client identity for the two aggregator CLIs whose gateways check who is
// calling. Both front a normal OpenAI-compatible endpoint, so no protocol work
// is needed — only the enrollment and the header overlay.

const (
	// Cline's OAuth access tokens are WorkOS JWTs and must be sent with a
	// "workos:" prefix; ClinePass API keys (clp_...) are not JWTs and must be
	// forwarded verbatim, or the API answers 401 "please re-authenticate".
	ClineTokenPrefix = "workos:"
	ClineReferer     = "https://cline.bot"
	ClineClientTitle = "Cline"
)

// ClineAccessToken renders the credential the Cline API expects.
func ClineAccessToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" || strings.HasPrefix(strings.ToLower(token), ClineTokenPrefix) {
		return token
	}
	// JWT header shape: eyJ<base64url>.<base64url>. Anything else (API key)
	// rides through unchanged.
	parts := strings.Split(token, ".")
	if len(parts) >= 2 && strings.HasPrefix(parts[0], "eyJ") {
		return ClineTokenPrefix + token
	}
	return token
}

// clineHeadersHook adds the editor identity Cline's gateway expects next to the
// bearer token.
func clineHeadersHook(h http.Header, c Credentials) {
	if h.Get("HTTP-Referer") == "" {
		h.Set("HTTP-Referer", ClineReferer)
	}
	if h.Get("X-Title") == "" {
		h.Set("X-Title", ClineClientTitle)
	}
}

// kilocodeOrgHook adds the organization the token belongs to. Kilo Code routes
// by org, so a multi-org account without this header is billed to the wrong
// one; single-org accounts simply omit it.
func kilocodeOrgHook(h http.Header, c Credentials) {
	if c.ProviderSpecificData == nil || h.Get("X-Kilocode-OrganizationID") != "" {
		return
	}
	if orgID, ok := c.ProviderSpecificData["orgId"].(string); ok && orgID != "" {
		h.Set("X-Kilocode-OrganizationID", orgID)
	}
}

func init() {
	authHooks["clineHeaders"] = clineHeadersHook
	authHooks["kilocodeOrg"] = kilocodeOrgHook
}

// decodeClineCode unwraps the base64 token bundle Cline's extension puts in the
// OAuth "code" parameter. Returns false when the code is opaque and has to be
// exchanged over the network instead.
func decodeClineCode(code string) (*TokenExchangeResult, bool) {
	raw := strings.TrimSpace(code)
	if raw == "" {
		return nil, false
	}
	if pad := len(raw) % 4; pad != 0 {
		raw += strings.Repeat("=", 4-pad)
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, false
	}
	text := string(decoded)
	if end := strings.LastIndex(text, "}"); end != -1 {
		text = text[:end+1]
	}
	var payload struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		Email        string `json:"email"`
		ExpiresAt    string `json:"expiresAt"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil || payload.AccessToken == "" {
		return nil, false
	}
	result := &TokenExchangeResult{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		Email:        payload.Email,
	}
	if payload.ExpiresAt != "" {
		if ts, err := time.Parse(time.RFC3339, payload.ExpiresAt); err == nil {
			result.ExpiresIn = int(time.Until(ts).Seconds())
		}
	}
	return result, true
}

// fetchKilocodeOrgID resolves the caller's first organization so chat and quota
// requests carry X-Kilocode-OrganizationID. Best-effort: a failure just means
// the account's default org is used.
func fetchKilocodeOrgID(token string, client *http.Client) string {
	if token == "" {
		return ""
	}
	if client == nil {
		client = SafeHTTPClient(15*time.Second, false)
	}
	req, err := http.NewRequest("GET", "https://api.kilo.ai/api/profile", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var payload struct {
		Organizations []struct {
			ID string `json:"id"`
		} `json:"organizations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ""
	}
	if len(payload.Organizations) == 0 {
		return ""
	}
	return payload.Organizations[0].ID
}
