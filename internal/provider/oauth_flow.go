package provider

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OAuthFlowType represents the type of OAuth flow a provider supports.
type OAuthFlowType string

const (
	FlowAuthorizationCodePKCE OAuthFlowType = "authorization_code_pkce"
	FlowAuthorizationCode     OAuthFlowType = "authorization_code"
	FlowDeviceCode            OAuthFlowType = "device_code"
	FlowImportToken           OAuthFlowType = "import_token"
)

// PKCE holds a PKCE code verifier/challenge pair and state.
type PKCE struct {
	CodeVerifier  string `json:"codeVerifier"`
	CodeChallenge string `json:"codeChallenge"`
	State         string `json:"state"`
}

// GeneratePKCE creates a new PKCE pair with S256 challenge method.
func GeneratePKCE() (*PKCE, error) {
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, fmt.Errorf("failed to generate code verifier: %w", err)
	}
	codeVerifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	h := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(h[:])

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	return &PKCE{
		CodeVerifier:  codeVerifier,
		CodeChallenge: codeChallenge,
		State:         state,
	}, nil
}

// OAuthSession stores in-flight OAuth session data (PKCE + redirect URI).
type OAuthSession struct {
	Provider    string
	PKCE        *PKCE
	RedirectURI string
	CreatedAt   time.Time
}

// sessionStore holds in-flight OAuth sessions keyed by state.
var sessionStore sync.Map

const sessionTTL = 10 * time.Minute

// StoreSession saves an OAuth session for later callback validation.
func StoreSession(state string, session *OAuthSession) {
	sessionStore.Store(state, session)
}

// GetSession retrieves and validates an OAuth session by state.
func GetSession(state string) (*OAuthSession, bool) {
	v, ok := sessionStore.Load(state)
	if !ok {
		return nil, false
	}
	session := v.(*OAuthSession)
	if time.Since(session.CreatedAt) > sessionTTL {
		sessionStore.Delete(state)
		return nil, false
	}
	return session, true
}

// ClearSession removes an OAuth session after use.
func ClearSession(state string) {
	sessionStore.Delete(state)
}

// GetProviderFlowType determines the OAuth flow type for a provider.
func GetProviderFlowType(providerID string) OAuthFlowType {
	info, ok := Registry[providerID]
	if !ok {
		return ""
	}
	if info.DeviceCodeURL != "" {
		return FlowDeviceCode
	}
	if info.AuthorizeURL != "" && info.ClientID != "" {
		return FlowAuthorizationCodePKCE
	}
	if info.AuthorizeURL != "" {
		return FlowAuthorizationCode
	}
	return FlowImportToken
}

// BuildAuthorizeURL constructs the authorization URL for a provider.
func BuildAuthorizeURL(providerID, redirectURI string, pkce *PKCE) (string, error) {
	info, ok := Registry[providerID]
	if !ok {
		return "", fmt.Errorf("unknown provider: %s", providerID)
	}
	if info.AuthorizeURL == "" {
		return "", fmt.Errorf("provider %s does not support authorization code flow", providerID)
	}

	params := url.Values{}

	switch providerID {
	case "claude":
		params.Set("code", "true")
		params.Set("client_id", info.ClientID)
		params.Set("response_type", "code")
		params.Set("redirect_uri", redirectURI)
		params.Set("scope", "org:create_api_key user:profile user:inference")
		params.Set("code_challenge", pkce.CodeChallenge)
		params.Set("code_challenge_method", "S256")
		params.Set("state", pkce.State)
	case "codex":
		params.Set("response_type", "code")
		params.Set("client_id", info.ClientID)
		params.Set("redirect_uri", redirectURI)
		params.Set("scope", "openid profile email offline_access")
		params.Set("code_challenge", pkce.CodeChallenge)
		params.Set("code_challenge_method", "S256")
		params.Set("state", pkce.State)
	case "gemini", "antigravity":
		params.Set("client_id", info.ClientID)
		params.Set("response_type", "code")
		params.Set("redirect_uri", redirectURI)
		params.Set("scope", "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email")
		params.Set("state", pkce.State)
		params.Set("access_type", "offline")
		params.Set("prompt", "consent")
	case "cline", "clinepass":
		params.Set("client_type", "extension")
		params.Set("callback_url", redirectURI)
		params.Set("redirect_uri", redirectURI)
	default:
		// Generic OAuth2 with PKCE
		params.Set("client_id", info.ClientID)
		params.Set("response_type", "code")
		params.Set("redirect_uri", redirectURI)
		params.Set("state", pkce.State)
		if pkce.CodeChallenge != "" {
			params.Set("code_challenge", pkce.CodeChallenge)
			params.Set("code_challenge_method", "S256")
		}
	}

	return info.AuthorizeURL + "?" + params.Encode(), nil
}

// TokenExchangeResult holds the result of a token exchange.
type TokenExchangeResult struct {
	AccessToken          string         `json:"accessToken"`
	RefreshToken         string         `json:"refreshToken,omitempty"`
	ExpiresIn            int            `json:"expiresIn,omitempty"`
	Email                string         `json:"email,omitempty"`
	DisplayName          string         `json:"displayName,omitempty"`
	ProviderSpecificData map[string]any `json:"providerSpecificData,omitempty"`
}

// ExchangeCode exchanges an authorization code for tokens.
func ExchangeCode(providerID, code, redirectURI, codeVerifier string, client *http.Client) (*TokenExchangeResult, error) {
	info, ok := Registry[providerID]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", providerID)
	}
	if info.TokenURL == "" {
		return nil, fmt.Errorf("provider %s has no token URL configured", providerID)
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	var resp *http.Response
	var err error

	switch providerID {
	case "claude":
		// Claude uses JSON body
		body := map[string]string{
			"code":          code,
			"grant_type":    "authorization_code",
			"client_id":     info.ClientID,
			"redirect_uri":  redirectURI,
			"code_verifier": codeVerifier,
		}
		bodyBytes, _ := json.Marshal(body)
		req, reqErr := http.NewRequest("POST", info.TokenURL, strings.NewReader(string(bodyBytes)))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err = client.Do(req)
	case "cline", "clinepass":
		// Cline encodes token data as base64 in the code param
		result, decodeErr := decodeClineToken(code)
		if decodeErr == nil {
			return result, nil
		}
		// Fallback to standard exchange
		params := url.Values{
			"grant_type":   {"authorization_code"},
			"code":         {code},
			"client_type":  {"extension"},
			"redirect_uri": {redirectURI},
		}
		req, reqErr := http.NewRequest("POST", info.TokenURL, strings.NewReader(params.Encode()))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err = client.Do(req)
	default:
		// Standard form-encoded exchange (codex, gemini, antigravity, etc.)
		params := url.Values{
			"grant_type":   {"authorization_code"},
			"code":         {code},
			"redirect_uri": {redirectURI},
		}
		if info.ClientID != "" {
			params.Set("client_id", info.ClientID)
		}
		if codeVerifier != "" {
			params.Set("code_verifier", codeVerifier)
		}
		req, reqErr := http.NewRequest("POST", info.TokenURL, strings.NewReader(params.Encode()))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err = client.Do(req)
	}

	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange returned status %d: %s", resp.StatusCode, string(body))
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return mapTokenResponse(providerID, raw), nil
}

// decodeClineToken attempts to decode a Cline base64-encoded token.
func decodeClineToken(code string) (*TokenExchangeResult, error) {
	// Add padding if needed
	b64 := code
	if pad := len(b64) % 4; pad != 0 {
		b64 += strings.Repeat("=", 4-pad)
	}
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	// Find last } to extract JSON
	s := string(decoded)
	lastBrace := strings.LastIndex(s, "}")
	if lastBrace == -1 {
		return nil, fmt.Errorf("no JSON found in decoded code")
	}
	var tokenData struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		Email        string `json:"email"`
		FirstName    string `json:"firstName"`
		LastName     string `json:"lastName"`
		ExpiresAt    string `json:"expiresAt"`
	}
	if err := json.Unmarshal([]byte(s[:lastBrace+1]), &tokenData); err != nil {
		return nil, err
	}
	if tokenData.AccessToken == "" {
		return nil, fmt.Errorf("no access token in decoded data")
	}

	expiresIn := 3600
	if tokenData.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, tokenData.ExpiresAt); err == nil {
			expiresIn = int(time.Until(t).Seconds())
		}
	}

	return &TokenExchangeResult{
		AccessToken:  tokenData.AccessToken,
		RefreshToken: tokenData.RefreshToken,
		ExpiresIn:    expiresIn,
		Email:        tokenData.Email,
		ProviderSpecificData: map[string]any{
			"firstName": tokenData.FirstName,
			"lastName":  tokenData.LastName,
		},
	}, nil
}

// DeviceCodeResponse holds the response from a device code request.
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
	CodeVerifier            string `json:"codeVerifier,omitempty"`
	// Extra data for polling (provider-specific)
	ExtraData map[string]any `json:"extraData,omitempty"`
}

// RequestDeviceCode initiates a device code flow for a provider.
func RequestDeviceCode(providerID string, client *http.Client) (*DeviceCodeResponse, error) {
	info, ok := Registry[providerID]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", providerID)
	}
	if info.DeviceCodeURL == "" {
		return nil, fmt.Errorf("provider %s does not support device code flow", providerID)
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	var pkce *PKCE
	// Qwen uses PKCE with device code
	if providerID == "qwen" {
		var err error
		pkce, err = GeneratePKCE()
		if err != nil {
			return nil, err
		}
	}

	var resp *http.Response
	var err error

	switch providerID {
	case "github":
		params := url.Values{
			"client_id": {info.ClientID},
			"scope":     {"read:user"},
		}
		req, reqErr := http.NewRequest("POST", info.DeviceCodeURL, strings.NewReader(params.Encode()))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err = client.Do(req)

	case "qwen":
		params := url.Values{
			"client_id":             {info.ClientID},
			"scope":                 {"openid profile email offline_access"},
			"code_challenge":        {pkce.CodeChallenge},
			"code_challenge_method": {"S256"},
		}
		req, reqErr := http.NewRequest("POST", info.DeviceCodeURL, strings.NewReader(params.Encode()))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err = client.Do(req)

	case "kimi":
		deviceID := generateDeviceID()
		params := url.Values{
			"client_id": {info.ClientID},
		}
		req, reqErr := http.NewRequest("POST", info.DeviceCodeURL, strings.NewReader(params.Encode()))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Msh-Device-Id", deviceID)
		resp, err = client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("device code request failed: %s", string(body))
		}
		var data map[string]any
		json.NewDecoder(resp.Body).Decode(&data)
		interval := 5
		if v, ok := data["interval"].(float64); ok {
			interval = int(v)
		}
		expiresIn := 300
		if v, ok := data["expires_in"].(float64); ok {
			expiresIn = int(v)
		}
		verifURI, _ := data["verification_uri"].(string)
		if verifURI == "" {
			verifURI = "https://www.kimi.com/code/authorize_device"
		}
		userCode, _ := data["user_code"].(string)
		return &DeviceCodeResponse{
			DeviceCode:              getString(data, "device_code"),
			UserCode:                userCode,
			VerificationURI:         verifURI,
			VerificationURIComplete: verifURI + "?user_code=" + userCode,
			ExpiresIn:               expiresIn,
			Interval:                interval,
			ExtraData:               map[string]any{"deviceId": deviceID},
		}, nil

	case "grok-cli":
		params := url.Values{
			"client_id": {info.ClientID},
			"scope":     {"openid profile email offline_access"},
		}
		req, reqErr := http.NewRequest("POST", info.DeviceCodeURL, strings.NewReader(params.Encode()))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err = client.Do(req)

	default:
		// Generic device code request
		params := url.Values{
			"client_id": {info.ClientID},
		}
		req, reqErr := http.NewRequest("POST", info.DeviceCodeURL, strings.NewReader(params.Encode()))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err = client.Do(req)
	}

	if err != nil {
		return nil, fmt.Errorf("device code request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device code request returned status %d: %s", resp.StatusCode, string(body))
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode device code response: %w", err)
	}

	interval := 5
	if v, ok := data["interval"].(float64); ok {
		interval = int(v)
	}
	expiresIn := 300
	if v, ok := data["expires_in"].(float64); ok {
		expiresIn = int(v)
	}

	result := &DeviceCodeResponse{
		DeviceCode:              getString(data, "device_code"),
		UserCode:                getString(data, "user_code"),
		VerificationURI:         getString(data, "verification_uri"),
		VerificationURIComplete: getString(data, "verification_uri_complete"),
		ExpiresIn:               expiresIn,
		Interval:                interval,
	}

	if pkce != nil {
		result.CodeVerifier = pkce.CodeVerifier
	}

	return result, nil
}

// PollDeviceCodeResult holds the result of polling for a device code token.
type PollDeviceCodeResult struct {
	Success bool                 `json:"success"`
	Pending bool                 `json:"pending"`
	Error   string               `json:"error,omitempty"`
	Tokens  *TokenExchangeResult `json:"tokens,omitempty"`
}

// PollDeviceCode polls the token endpoint for a device code flow.
func PollDeviceCode(providerID, deviceCode, codeVerifier string, extraData map[string]any, client *http.Client) (*PollDeviceCodeResult, error) {
	info, ok := Registry[providerID]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", providerID)
	}
	if info.TokenURL == "" {
		return nil, fmt.Errorf("provider %s has no token URL configured", providerID)
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	var resp *http.Response
	var err error

	switch providerID {
	case "github":
		params := url.Values{
			"client_id":   {info.ClientID},
			"device_code": {deviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}
		req, reqErr := http.NewRequest("POST", info.TokenURL, strings.NewReader(params.Encode()))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err = client.Do(req)

	case "qwen":
		params := url.Values{
			"grant_type":    {"urn:ietf:params:oauth:grant-type:device_code"},
			"client_id":     {info.ClientID},
			"device_code":   {deviceCode},
			"code_verifier": {codeVerifier},
		}
		req, reqErr := http.NewRequest("POST", info.TokenURL, strings.NewReader(params.Encode()))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err = client.Do(req)

	case "kimi":
		deviceID := ""
		if extraData != nil {
			deviceID, _ = extraData["deviceId"].(string)
		}
		params := url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"client_id":   {info.ClientID},
			"device_code": {deviceCode},
		}
		req, reqErr := http.NewRequest("POST", info.TokenURL, strings.NewReader(params.Encode()))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		if deviceID != "" {
			req.Header.Set("X-Msh-Device-Id", deviceID)
		}
		resp, err = client.Do(req)

	case "grok-cli":
		params := url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"device_code": {deviceCode},
			"client_id":   {info.ClientID},
		}
		req, reqErr := http.NewRequest("POST", info.TokenURL, strings.NewReader(params.Encode()))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err = client.Do(req)

	default:
		params := url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"device_code": {deviceCode},
			"client_id":   {info.ClientID},
		}
		if codeVerifier != "" {
			params.Set("code_verifier", codeVerifier)
		}
		req, reqErr := http.NewRequest("POST", info.TokenURL, strings.NewReader(params.Encode()))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err = client.Do(req)
	}

	if err != nil {
		return nil, fmt.Errorf("poll request failed: %w", err)
	}
	defer resp.Body.Close()

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode poll response: %w", err)
	}

	// Check for pending/error states
	errCode := getString(data, "error")
	if errCode == "authorization_pending" || errCode == "slow_down" {
		return &PollDeviceCodeResult{Success: false, Pending: true, Error: errCode}, nil
	}
	if errCode != "" && resp.StatusCode != http.StatusOK {
		errDesc := getString(data, "error_description")
		return &PollDeviceCodeResult{Success: false, Pending: false, Error: errCode + ": " + errDesc}, nil
	}

	// Check for access token
	accessToken := getString(data, "access_token")
	if accessToken == "" {
		return &PollDeviceCodeResult{Success: false, Pending: true, Error: "authorization_pending"}, nil
	}

	tokens := mapTokenResponse(providerID, data)

	// Preserve device ID for kimi
	if providerID == "kimi" && extraData != nil {
		if deviceID, ok := extraData["deviceId"].(string); ok && deviceID != "" {
			if tokens.ProviderSpecificData == nil {
				tokens.ProviderSpecificData = make(map[string]any)
			}
			tokens.ProviderSpecificData["deviceId"] = deviceID
		}
	}

	return &PollDeviceCodeResult{Success: true, Tokens: tokens}, nil
}

// ImportToken creates a connection from a manually pasted token.
func ImportToken(providerID, accessToken string) (*TokenExchangeResult, error) {
	if accessToken == "" {
		return nil, fmt.Errorf("access token is required")
	}

	result := &TokenExchangeResult{
		AccessToken:          accessToken,
		ProviderSpecificData: map[string]any{"authMethod": "access_token"},
	}

	// Try to extract info from JWT
	if strings.HasPrefix(accessToken, "eyJ") && strings.Contains(accessToken, ".") {
		parts := strings.Split(accessToken, ".")
		if len(parts) >= 2 {
			payload, err := decodeJWTPayload(parts[1])
			if err == nil {
				if email, ok := payload["email"].(string); ok {
					result.Email = email
				}
				if preferredUsername, ok := payload["preferred_username"].(string); ok && result.Email == "" {
					result.Email = preferredUsername
				}
				// OpenAI-specific claims
				if auth, ok := payload["https://api.openai.com/auth"].(map[string]any); ok {
					if accountID, ok := auth["chatgpt_account_id"].(string); ok {
						result.ProviderSpecificData["chatgptAccountId"] = accountID
					}
					if planType, ok := auth["chatgpt_plan_type"].(string); ok {
						result.ProviderSpecificData["chatgptPlanType"] = planType
					}
				}
				if profile, ok := payload["https://api.openai.com/profile"].(map[string]any); ok {
					if email, ok := profile["email"].(string); ok && result.Email == "" {
						result.Email = email
					}
				}
				// Direct account_id/plan_type (ChatGPT website tokens)
				if accountID, ok := payload["account_id"].(string); ok {
					result.ProviderSpecificData["chatgptAccountId"] = accountID
				}
				if planType, ok := payload["plan_type"].(string); ok {
					result.ProviderSpecificData["chatgptPlanType"] = planType
				}
				if exp, ok := payload["exp"].(float64); ok {
					result.ProviderSpecificData["jwtExp"] = int64(exp)
					result.ExpiresIn = int(time.Until(time.Unix(int64(exp), 0)).Seconds())
				}
			}
		}
	}

	return result, nil
}

// mapTokenResponse maps a raw token response to TokenExchangeResult.
func mapTokenResponse(providerID string, raw map[string]any) *TokenExchangeResult {
	result := &TokenExchangeResult{
		AccessToken:  getString(raw, "access_token"),
		RefreshToken: getString(raw, "refresh_token"),
	}

	if v, ok := raw["expires_in"].(float64); ok {
		result.ExpiresIn = int(v)
	}

	// Provider-specific email extraction
	switch providerID {
	case "codex":
		if idToken := getString(raw, "id_token"); idToken != "" {
			if payload, err := decodeJWTPayload(strings.Split(idToken, ".")[1]); err == nil {
				if auth, ok := payload["https://api.openai.com/auth"].(map[string]any); ok {
					if accountID, ok := auth["chatgpt_account_id"].(string); ok {
						if result.ProviderSpecificData == nil {
							result.ProviderSpecificData = make(map[string]any)
						}
						result.ProviderSpecificData["chatgptAccountId"] = accountID
					}
				}
				if profile, ok := payload["https://api.openai.com/profile"].(map[string]any); ok {
					if email, ok := profile["email"].(string); ok {
						result.Email = email
					}
				}
			}
		}
	case "github":
		// GitHub doesn't return email in token response; would need separate API call
		result.ProviderSpecificData = map[string]any{"authMethod": "device_code"}
	case "grok-cli":
		result.ProviderSpecificData = map[string]any{"authMethod": "device_code"}
		if idToken := getString(raw, "id_token"); idToken != "" {
			if payload, err := decodeJWTPayload(strings.Split(idToken, ".")[1]); err == nil {
				if email, ok := payload["email"].(string); ok {
					result.Email = email
				}
			}
		}
	case "kimi":
		result.ProviderSpecificData = map[string]any{"authMethod": "device_code"}
	}

	return result
}

// decodeJWTPayload decodes the payload (second part) of a JWT.
func decodeJWTPayload(b64 string) (map[string]any, error) {
	// Convert base64url to standard base64
	b64 = strings.ReplaceAll(b64, "-", "+")
	b64 = strings.ReplaceAll(b64, "_", "/")
	// Add padding
	if pad := len(b64) % 4; pad != 0 {
		b64 += strings.Repeat("=", 4-pad)
	}
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// generateDeviceID creates a random device ID for providers that need one.
func generateDeviceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// getString safely extracts a string value from a map.
func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
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
			// Spin-wait briefly (refresh should be fast)
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
			// Timeout waiting, do our own refresh
		}
	}

	// Create new entry
	entry := &refreshEntry{}
	entry.mu.Lock()
	actual, loaded := refreshDedupMap.LoadOrStore(key, entry)
	if loaded {
		// Another goroutine beat us, wait on their entry
		existing := actual.(*refreshEntry)
		entry.mu.Unlock()
		existing.mu.Lock()
		if existing.done {
			result, err := existing.result, existing.err
			existing.mu.Unlock()
			return result, err
		}
		existing.mu.Unlock()
		// Fall through to do our own refresh
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
