package provider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// RefreshLeadTime is the default pre-expiry buffer for proactive refresh.
const RefreshLeadTime = 5 * time.Minute

// Per-provider refresh lead overrides (mirrors 9router REFRESH_LEAD_MS).
var refreshLeadOverrides = map[string]time.Duration{
	"codex":  5 * 24 * time.Hour, // 432000000ms — codex tokens are long-lived
	"iflow":  24 * time.Hour,     // 86400000ms
	"kimi":   5 * time.Minute,
	"xai":    5 * time.Minute,
	"qwen":   5 * time.Minute,
	"google": 5 * time.Minute,
}

// GetRefreshLead returns the pre-expiry buffer for a provider.
func GetRefreshLead(providerID string) time.Duration {
	if d, ok := refreshLeadOverrides[providerID]; ok {
		return d
	}
	return RefreshLeadTime
}

// ShouldRefresh checks if a connection's token is about to expire.
func ShouldRefresh(conn *model.ProviderConnection) bool {
	if conn.Data.RefreshToken == "" {
		return false
	}
	if conn.Data.ExpiresAt == "" {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339, conn.Data.ExpiresAt)
	if err != nil {
		return false
	}
	lead := GetRefreshLead(conn.Provider)
	return time.Until(expiresAt) < lead
}

// RefreshResult holds the outcome of a token refresh.
type RefreshResult struct {
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	ExpiresIn    int            `json:"expires_in"`
	IDToken      string         `json:"id_token,omitempty"`
	Extra        map[string]any `json:"extra,omitempty"`
}

// --- Error Classification ---

// RefreshError classifies an OAuth refresh failure.
type RefreshError struct {
	Status      int
	Code        string
	Description string
	Permanent   bool // true = re-auth required, token is unrecoverable
}

func (e *RefreshError) Error() string {
	return fmt.Sprintf("oauth refresh failed (status=%d, code=%s, permanent=%v): %s", e.Status, e.Code, e.Permanent, e.Description)
}

// ClassifyRefreshError determines if a refresh failure is permanent (re-auth needed).
func ClassifyRefreshError(body string, status int) *RefreshError {
	var parsed map[string]any
	if body != "" {
		json.Unmarshal([]byte(body), &parsed)
	}

	code := ""
	description := body
	if parsed != nil {
		if v, ok := parsed["error"].(string); ok {
			code = v
		}
		if v, ok := parsed["error_code"].(string); ok && code == "" {
			code = v
		}
		if v, ok := parsed["error_description"].(string); ok {
			description = v
		} else if v, ok := parsed["message"].(string); ok {
			description = v
		}
	}

	combined := strings.ToLower(code + " " + description)
	permanent := strings.Contains(combined, "refresh_token_expired") ||
		strings.Contains(combined, "refresh_token_reused") ||
		strings.Contains(combined, "refresh_token_invalidated") ||
		strings.Contains(combined, "invalid_grant")

	return &RefreshError{
		Status:      status,
		Code:        code,
		Description: description,
		Permanent:   permanent,
	}
}

// IsUnrecoverableRefreshError checks if a refresh result indicates permanent failure.
func IsUnrecoverableRefreshError(err error) bool {
	if re, ok := err.(*RefreshError); ok {
		return re.Permanent
	}
	return false
}

// --- Per-Provider Refresh Profiles ---

// refreshProfile describes how a provider's token refresh request is built.
type refreshProfile struct {
	// URL override; if empty, uses registry TokenURL.
	url string
	// bodyFormat: "form" (default) or "json".
	bodyFormat string
	// includeClientSecret adds client_secret to the body.
	includeClientSecret bool
	// clientSecret value (for providers with static secrets).
	clientSecret string
	// extraHeaders returns additional headers for the request.
	extraHeaders func(conn *model.ProviderConnection) map[string]string
	// parse extracts extra data from the token response.
	parse func(raw map[string]any) map[string]any
}

var refreshProfiles = map[string]refreshProfile{
	"claude": {
		url:        "https://console.anthropic.com/v1/oauth/token",
		bodyFormat: "json",
	},
	"kimi": {
		extraHeaders: kimiRefreshHeaders,
	},
	"xai": {
		// xAI uses OIDC discovery; URL resolved dynamically.
	},
	"codex": {
		bodyFormat: "json",
	},
	"qwen": {
		parse: func(raw map[string]any) map[string]any {
			if ru, ok := raw["resource_url"].(string); ok && ru != "" {
				return map[string]any{"resourceUrl": ru}
			}
			return nil
		},
	},
	"iflow": {
		clientSecret: "4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW",
		extraHeaders: func(conn *model.ProviderConnection) map[string]string {
			info, ok := Registry["iflow"]
			if !ok {
				return nil
			}
			creds := base64.StdEncoding.EncodeToString([]byte(info.ClientID + ":4Z3YjXycVsQvyGF1etiNlIBB4RsqSDtW"))
			return map[string]string{"Authorization": "Basic " + creds}
		},
	},
	"gemini": {
		clientSecret: "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",
	},
	"gemini-cli": {
		clientSecret: "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",
	},
	"kiro": {
		// Kiro has multi-path refresh handled separately.
	},
}

// kimiRefreshHeaders builds X-Msh-* headers for kimi token refresh.
func kimiRefreshHeaders(conn *model.ProviderConnection) map[string]string {
	deviceID := ""
	if conn.Data.ProviderSpecificData != nil {
		deviceID, _ = conn.Data.ProviderSpecificData["deviceId"].(string)
	}
	if deviceID == "" {
		deviceID = "kimi-" + fmt.Sprintf("%d", time.Now().UnixMilli())
	}

	osName := runtime.GOOS
	archName := runtime.GOARCH
	var deviceModel string
	switch osName {
	case "darwin":
		deviceModel = "macOS " + archName
	case "windows":
		deviceModel = "Windows " + archName
	default:
		deviceModel = "Linux " + archName
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	return map[string]string{
		"X-Msh-Platform":     "cyrene-gateway",
		"X-Msh-Version":      "1.0.0",
		"X-Msh-Device-Name":  hostname,
		"X-Msh-Device-Model": deviceModel,
		"X-Msh-Device-Id":    deviceID,
	}
}

// --- xAI OIDC Discovery ---

var (
	xaiDiscoveryOnce sync.Once
	xaiTokenURL      string
)

func discoverXaiTokenURL() string {
	xaiDiscoveryOnce.Do(func() {
		xaiTokenURL = "https://auth.x.ai/oauth2/token" // static fallback
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get("https://auth.x.ai/.well-known/openid-configuration")
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return
		}
		var data struct {
			TokenEndpoint string `json:"token_endpoint"`
		}
		if json.NewDecoder(resp.Body).Decode(&data) == nil && data.TokenEndpoint != "" {
			if strings.Contains(data.TokenEndpoint, "x.ai") {
				xaiTokenURL = data.TokenEndpoint
			}
		}
	})
	return xaiTokenURL
}

// --- Core Refresh Engine ---

// RefreshCredentials attempts to refresh the OAuth token for a connection.
// Dispatches to per-provider logic based on the provider ID.
func RefreshCredentials(providerID string, conn *model.ProviderConnection, client *http.Client) (*RefreshResult, error) {
	if conn.Data.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	// Kiro has a completely different multi-path refresh.
	if providerID == "kiro" {
		return refreshKiro(conn, client)
	}

	info, ok := Registry[providerID]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", providerID)
	}

	profile := refreshProfiles[providerID]

	// Resolve token URL.
	tokenURL := profile.url
	if tokenURL == "" {
		tokenURL = info.TokenURL
	}
	if providerID == "xai" || providerID == "grok-cli" {
		tokenURL = discoverXaiTokenURL()
	}
	if tokenURL == "" {
		return nil, fmt.Errorf("no token URL configured for provider: %s", providerID)
	}

	// Build request body.
	bodyFormat := profile.bodyFormat
	if bodyFormat == "" {
		bodyFormat = "form"
	}

	var reqBody io.Reader
	var contentType string

	switch bodyFormat {
	case "json":
		payload := map[string]string{
			"grant_type":    "refresh_token",
			"refresh_token": conn.Data.RefreshToken,
			"client_id":     info.ClientID,
		}
		if profile.includeClientSecret && profile.clientSecret != "" {
			payload["client_secret"] = profile.clientSecret
		}
		bodyBytes, _ := json.Marshal(payload)
		reqBody = strings.NewReader(string(bodyBytes))
		contentType = "application/json"
	default: // "form"
		params := url.Values{
			"grant_type":    {"refresh_token"},
			"refresh_token": {conn.Data.RefreshToken},
		}
		if info.ClientID != "" {
			params.Set("client_id", info.ClientID)
		}
		if profile.includeClientSecret && profile.clientSecret != "" {
			params.Set("client_secret", profile.clientSecret)
		}
		// Google needs client_secret
		if profile.clientSecret != "" && (providerID == "gemini" || providerID == "gemini-cli" || providerID == "antigravity") {
			params.Set("client_secret", profile.clientSecret)
		}
		reqBody = strings.NewReader(params.Encode())
		contentType = "application/x-www-form-urlencoded"
	}

	req, err := http.NewRequest("POST", tokenURL, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	// Apply extra headers (kimi X-Msh-*, iflow Basic auth).
	if profile.extraHeaders != nil {
		for k, v := range profile.extraHeaders(conn) {
			req.Header.Set(k, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		refreshErr := ClassifyRefreshError(string(respBody), resp.StatusCode)
		if refreshErr.Permanent {
			slog.Error("OAuth refresh permanently failed — re-auth required",
				slog.String("provider", providerID),
				slog.String("code", refreshErr.Code),
			)
		}
		return nil, refreshErr
	}

	var raw map[string]any
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("failed to decode refresh response: %w", err)
	}

	accessToken, _ := raw["access_token"].(string)
	if accessToken == "" {
		// Kiro social returns accessToken (camelCase)
		accessToken, _ = raw["accessToken"].(string)
	}
	if accessToken == "" {
		return nil, fmt.Errorf("refresh response missing access_token")
	}

	result := &RefreshResult{
		AccessToken: accessToken,
	}

	if rt, _ := raw["refresh_token"].(string); rt != "" {
		result.RefreshToken = rt
	} else if rt, _ := raw["refreshToken"].(string); rt != "" {
		result.RefreshToken = rt
	} else {
		result.RefreshToken = conn.Data.RefreshToken
	}

	if v, ok := raw["expires_in"].(float64); ok {
		result.ExpiresIn = int(v)
	} else if v, ok := raw["expiresIn"].(float64); ok {
		result.ExpiresIn = int(v)
	}

	if idt, _ := raw["id_token"].(string); idt != "" {
		result.IDToken = idt
	}

	// Provider-specific extra data extraction.
	if profile.parse != nil {
		result.Extra = profile.parse(raw)
	}

	return result, nil
}

// refreshKiro handles Kiro's multi-path token refresh (social, IDC, external_idp).
func refreshKiro(conn *model.ProviderConnection, client *http.Client) (*RefreshResult, error) {
	psd := conn.Data.ProviderSpecificData
	if psd == nil {
		psd = map[string]any{}
	}

	authMethod, _ := psd["authMethod"].(string)
	clientID, _ := psd["clientId"].(string)
	clientSecret, _ := psd["clientSecret"].(string)
	region, _ := psd["region"].(string)

	// Path 1: IDC (AWS OIDC) — clientId + clientSecret in JSON body.
	if clientID != "" && clientSecret != "" {
		endpoint := "https://oidc.us-east-1.amazonaws.com/token"
		if authMethod == "idc" && region != "" {
			endpoint = fmt.Sprintf("https://oidc.%s.amazonaws.com/token", region)
		}

		body, _ := json.Marshal(map[string]string{
			"clientId":     clientID,
			"clientSecret": clientSecret,
			"refreshToken": conn.Data.RefreshToken,
			"grantType":    "refresh_token",
		})

		req, err := http.NewRequest("POST", endpoint, strings.NewReader(string(body)))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("kiro IDC refresh failed: %w", err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return nil, ClassifyRefreshError(string(respBody), resp.StatusCode)
		}

		var data struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			ExpiresIn    int    `json:"expiresIn"`
			ProfileArn   string `json:"profileArn"`
		}
		if err := json.Unmarshal(respBody, &data); err != nil {
			return nil, fmt.Errorf("kiro IDC decode failed: %w", err)
		}
		if data.AccessToken == "" {
			return nil, fmt.Errorf("kiro IDC refresh returned no accessToken")
		}

		rt := data.RefreshToken
		if rt == "" {
			rt = conn.Data.RefreshToken
		}

		result := &RefreshResult{
			AccessToken:  data.AccessToken,
			RefreshToken: rt,
			ExpiresIn:    data.ExpiresIn,
		}
		// Patch profileArn if missing.
		if data.ProfileArn != "" {
			if _, hasArn := psd["profileArn"]; !hasArn {
				result.Extra = map[string]any{"profileArn": data.ProfileArn}
			}
		}
		return result, nil
	}

	// Path 2: Social auth (Kiro desktop) — JSON body with refreshToken only.
	socialURL := "https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken"
	body, _ := json.Marshal(map[string]string{
		"refreshToken": conn.Data.RefreshToken,
	})

	req, err := http.NewRequest("POST", socialURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "kiro-cli/1.0.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kiro social refresh failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, ClassifyRefreshError(string(respBody), resp.StatusCode)
	}

	var data struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int    `json:"expiresIn"`
		ProfileArn   string `json:"profileArn"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, fmt.Errorf("kiro social decode failed: %w", err)
	}
	if data.AccessToken == "" {
		return nil, fmt.Errorf("kiro social refresh returned no accessToken")
	}

	rt := data.RefreshToken
	if rt == "" {
		rt = conn.Data.RefreshToken
	}

	result := &RefreshResult{
		AccessToken:  data.AccessToken,
		RefreshToken: rt,
		ExpiresIn:    data.ExpiresIn,
	}
	if data.ProfileArn != "" {
		if _, hasArn := psd["profileArn"]; !hasArn {
			result.Extra = map[string]any{"profileArn": data.ProfileArn}
		}
	}
	return result, nil
}

// ApplyRefreshResult updates connection data with refreshed credentials.
func ApplyRefreshResult(conn *model.ProviderConnection, result *RefreshResult) {
	conn.Data.AccessToken = result.AccessToken
	conn.Data.RefreshToken = result.RefreshToken
	if result.ExpiresIn > 0 {
		conn.Data.ExpiresAt = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}
	// Merge extra data into ProviderSpecificData.
	if result.Extra != nil {
		if conn.Data.ProviderSpecificData == nil {
			conn.Data.ProviderSpecificData = make(map[string]any)
		}
		for k, v := range result.Extra {
			conn.Data.ProviderSpecificData[k] = v
		}
	}
	slog.Info("Token refreshed",
		slog.String("provider", conn.Provider),
		slog.String("connection", conn.ID),
	)
}

// --- Token Refresh Dedup Lock ---

type refreshEntry struct {
	mu        sync.Mutex
	result    *RefreshResult
	err       error
	expiresAt time.Time
	done      bool
}

var refreshDedupMap sync.Map

const refreshResultTTL = 10 * time.Second

// DedupRefresh ensures only one refresh runs per provider+token combination.
// Concurrent callers with the same key will wait for the first refresh to complete.
func DedupRefresh(providerID string, oldToken string, fn func() (*RefreshResult, error)) (*RefreshResult, error) {
	if oldToken == "" {
		return fn()
	}
	key := providerID + ":" + oldToken

	// Check for cached result
	if v, ok := refreshDedupMap.Load(key); ok {
		entry := v.(*refreshEntry)
		entry.mu.Lock()
		if entry.done {
			if time.Now().Before(entry.expiresAt) {
				result, err := entry.result, entry.err
				entry.mu.Unlock()
				return result, err
			}
			// Expired, delete and proceed
			entry.mu.Unlock()
			refreshDedupMap.Delete(key)
		} else {
			// Wait for in-flight refresh
			entry.mu.Unlock()
			for i := 0; i < 100; i++ {
				time.Sleep(100 * time.Millisecond)
				entry.mu.Lock()
				if entry.done {
					result, err := entry.result, entry.err
					entry.mu.Unlock()
					return result, err
				}
				entry.mu.Unlock()
			}
		}
	}

	// Create new entry
	entry := &refreshEntry{}
	entry.mu.Lock()
	actual, loaded := refreshDedupMap.LoadOrStore(key, entry)
	if loaded {
		existing := actual.(*refreshEntry)
		entry.mu.Unlock()
		existing.mu.Lock()
		if existing.done {
			result, err := existing.result, existing.err
			existing.mu.Unlock()
			return result, err
		}
		existing.mu.Unlock()
	}

	// Execute the refresh
	result, err := fn()

	entry.result = result
	entry.err = err
	entry.expiresAt = time.Now().Add(refreshResultTTL)
	entry.done = true
	entry.mu.Unlock()

	if err != nil {
		refreshDedupMap.Delete(key)
	}

	return result, err
}
