package model

import "time"

// ConnectionDTO represents a redacted provider connection for API responses.
type ConnectionDTO struct {
	ID        string            `json:"id"`
	Provider  string            `json:"provider"`
	AuthType  string            `json:"authType"`
	Name      string            `json:"name,omitempty"`
	Email     string            `json:"email,omitempty"`
	Priority  int               `json:"priority"`
	IsActive  bool              `json:"isActive"`
	Data      ConnectionDataDTO `json:"data"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

type ConnectionDataDTO struct {
	HasAPIKey            bool           `json:"hasApiKey"`
	HasAccessToken       bool           `json:"hasAccessToken"`
	HasRefreshToken      bool           `json:"hasRefreshToken"`
	CredentialHint       string         `json:"credentialHint,omitempty"`
	ExpiresAt            string         `json:"expiresAt,omitempty"`
	BaseURL              string         `json:"baseUrl,omitempty"`
	TestStatus           string         `json:"testStatus,omitempty"`
	LastError            string         `json:"lastError,omitempty"`
	RateLimitedUntil     string         `json:"rateLimitedUntil,omitempty"`
	BackoffLevel         int            `json:"backoffLevel,omitempty"`
	QuotaLimit           int            `json:"quotaLimit,omitempty"`
	QuotaPeriod          string         `json:"quotaPeriod,omitempty"`
	ProviderSpecificData map[string]any `json:"providerSpecificData,omitempty"`
}

// ToDTO converts a ProviderConnection to a redacted ConnectionDTO.
func (pc *ProviderConnection) ToDTO() ConnectionDTO {
	hint := ""
	if pc.Data.APIKey != "" {
		k := pc.Data.APIKey
		if len(k) > 4 {
			hint = "..." + k[len(k)-4:]
		} else {
			hint = "***"
		}
	} else if pc.Data.AccessToken != "" {
		k := pc.Data.AccessToken
		if len(k) > 4 {
			hint = "..." + k[len(k)-4:]
		} else {
			hint = "***"
		}
	}

	// Redact provider-specific secrets if any
	psdRedacted := make(map[string]any)
	for k, v := range pc.Data.ProviderSpecificData {
		// Filter known secret keys in providerSpecificData
		kLower := k
		if kLower == "refreshToken" || kLower == "clientSecret" || kLower == "secret" || kLower == "token" || kLower == "api_key" || kLower == "apiKey" {
			psdRedacted["has_"+k] = true
		} else {
			psdRedacted[k] = v
		}
	}

	return ConnectionDTO{
		ID:        pc.ID,
		Provider:  pc.Provider,
		AuthType:  pc.AuthType,
		Name:      pc.Name,
		Email:     pc.Email,
		Priority:  pc.Priority,
		IsActive:  pc.IsActive,
		CreatedAt: pc.CreatedAt,
		UpdatedAt: pc.UpdatedAt,
		Data: ConnectionDataDTO{
			HasAPIKey:            pc.Data.APIKey != "",
			HasAccessToken:       pc.Data.AccessToken != "",
			HasRefreshToken:      pc.Data.RefreshToken != "",
			CredentialHint:       hint,
			ExpiresAt:            pc.Data.ExpiresAt,
			BaseURL:              pc.Data.BaseURL,
			TestStatus:           pc.Data.TestStatus,
			LastError:            pc.Data.LastError,
			RateLimitedUntil:     pc.Data.RateLimitedUntil,
			BackoffLevel:         pc.Data.BackoffLevel,
			QuotaLimit:           pc.Data.QuotaLimit,
			QuotaPeriod:          pc.Data.QuotaPeriod,
			ProviderSpecificData: psdRedacted,
		},
	}
}
