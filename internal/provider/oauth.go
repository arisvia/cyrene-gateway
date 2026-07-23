package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// OAuth refresh configuration per provider.
type OAuthRefreshConfig struct {
	TokenURL    string
	ClientID    string
	Encoding    string // "form" or "json"
	ExtraParams map[string]string
}

// Known OAuth refresh endpoints (mirrors 9router PROVIDER_OAUTH).
var oauthRefreshConfigs = map[string]OAuthRefreshConfig{
	"claude": {
		TokenURL: "https://console.anthropic.com/v1/oauth/token",
		ClientID: "9d1c250a-e61b-44d9-88ed-5944d1962f5e",
		Encoding: "json",
	},
	"gemini": {
		TokenURL: "https://oauth2.googleapis.com/token",
		Encoding: "form",
	},
	"qwen": {
		TokenURL: "https://chat.qwenlm.ai/api/oauth/token",
		ClientID: "fOMeykt37MGEjXlBdCFO",
		Encoding: "form",
	},
}

// RefreshLeadTime is how long before expiry we proactively refresh.
const RefreshLeadTime = 5 * time.Minute

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
	return time.Until(expiresAt) < RefreshLeadTime
}

// RefreshResult holds the outcome of a token refresh.
type RefreshResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// RefreshCredentials attempts to refresh the OAuth token for a connection.
// Returns updated connection data fields or an error.
func RefreshCredentials(providerID string, conn *model.ProviderConnection, client *http.Client) (*RefreshResult, error) {
	cfg, ok := oauthRefreshConfigs[providerID]
	if !ok {
		return nil, fmt.Errorf("no OAuth refresh config for provider: %s", providerID)
	}
	if conn.Data.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	var resp *http.Response
	var err error

	switch cfg.Encoding {
	case "json":
		body := map[string]string{
			"grant_type":    "refresh_token",
			"refresh_token": conn.Data.RefreshToken,
			"client_id":     cfg.ClientID,
		}
		for k, v := range cfg.ExtraParams {
			body[k] = v
		}
		bodyBytes, _ := json.Marshal(body)
		req, reqErr := http.NewRequest("POST", cfg.TokenURL, strings.NewReader(string(bodyBytes)))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err = client.Do(req)
	default: // "form"
		params := url.Values{
			"grant_type":    {"refresh_token"},
			"refresh_token": {conn.Data.RefreshToken},
		}
		if cfg.ClientID != "" {
			params.Set("client_id", cfg.ClientID)
		}
		for k, v := range cfg.ExtraParams {
			params.Set(k, v)
		}
		req, reqErr := http.NewRequest("POST", cfg.TokenURL, strings.NewReader(params.Encode()))
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err = client.Do(req)
	}

	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("refresh returned status %d: %s", resp.StatusCode, string(body))
	}

	var result RefreshResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode refresh response: %w", err)
	}

	if result.AccessToken == "" {
		return nil, fmt.Errorf("refresh response missing access_token")
	}

	// Preserve old refresh token if not rotated
	if result.RefreshToken == "" {
		result.RefreshToken = conn.Data.RefreshToken
	}

	return &result, nil
}

// ApplyRefreshResult updates connection data with refreshed credentials.
func ApplyRefreshResult(conn *model.ProviderConnection, result *RefreshResult) {
	conn.Data.AccessToken = result.AccessToken
	conn.Data.RefreshToken = result.RefreshToken
	if result.ExpiresIn > 0 {
		conn.Data.ExpiresAt = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}
	slog.Info("Token refreshed",
		slog.String("provider", conn.Provider),
		slog.String("connection", conn.ID),
	)
}
