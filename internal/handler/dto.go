package handler

import (
	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"time"
)

// Response DTOs are separated from persistence models: provider credentials
// and other secrets are never serialized into management API responses
// (37A / P0-1). Only presence flags and masked hints are exposed.

// ConnectionDTO is the redacted management API representation of a provider
// connection.
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

// ConnectionDataDTO carries only non-secret connection state.
type ConnectionDataDTO struct {
	HasAPIKey        bool   `json:"hasApiKey"`
	HasAccessToken   bool   `json:"hasAccessToken"`
	HasRefreshToken  bool   `json:"hasRefreshToken"`
	CredentialHint   string `json:"credentialHint,omitempty"`
	APIKeyHint       string `json:"apiKeyHint,omitempty"`
	AccessTokenHint  string `json:"accessTokenHint,omitempty"`
	BaseURL          string `json:"baseUrl,omitempty"`
	ExpiresAt        string `json:"expiresAt,omitempty"`
	TestStatus       string `json:"testStatus,omitempty"`
	LastError        string `json:"lastError,omitempty"`
	RateLimitedUntil string `json:"rateLimitedUntil,omitempty"`
	BackoffLevel     int    `json:"backoffLevel,omitempty"`
	QuotaLimit       int    `json:"quotaLimit,omitempty"`
	QuotaPeriod      string `json:"quotaPeriod,omitempty"`
}

// toConnectionDTO converts a persistence model into its redacted DTO.
// providerSpecificData is intentionally omitted: it is provider-internal and
// may carry sensitive values.
func toConnectionDTO(c *model.ProviderConnection) ConnectionDTO {
	d := c.Data
	dto := ConnectionDTO{
		ID:        c.ID,
		Provider:  c.Provider,
		AuthType:  c.AuthType,
		Name:      c.Name,
		Email:     c.Email,
		Priority:  c.Priority,
		IsActive:  c.IsActive,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
		Data: ConnectionDataDTO{
			HasAPIKey:        d.APIKey != "",
			HasAccessToken:   d.AccessToken != "",
			HasRefreshToken:  d.RefreshToken != "",
			APIKeyHint:       auth.MaskSecret(d.APIKey),
			AccessTokenHint:  auth.MaskSecret(d.AccessToken),
			BaseURL:          d.BaseURL,
			ExpiresAt:        d.ExpiresAt,
			TestStatus:       d.TestStatus,
			LastError:        d.LastError,
			RateLimitedUntil: d.RateLimitedUntil,
			BackoffLevel:     d.BackoffLevel,
			QuotaLimit:       d.QuotaLimit,
			QuotaPeriod:      d.QuotaPeriod,
		},
	}
	switch {
	case d.APIKey != "":
		dto.Data.CredentialHint = dto.Data.APIKeyHint
	case d.AccessToken != "":
		dto.Data.CredentialHint = dto.Data.AccessTokenHint
	}
	return dto
}

func toConnectionDTOList(conns []model.ProviderConnection) []ConnectionDTO {
	out := make([]ConnectionDTO, 0, len(conns))
	for i := range conns {
		out = append(out, toConnectionDTO(&conns[i]))
	}
	return out
}

// APIKeyDTO is the redacted list representation of a local client API key.
// The full key is only returned once, in the create response.
type APIKeyDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name,omitempty"`
	KeyHint   string    `json:"keyHint"`
	IsActive  bool      `json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
}

func toAPIKeyDTO(k *model.APIKey) APIKeyDTO {
	return APIKeyDTO{
		ID:        k.ID,
		Name:      k.Name,
		KeyHint:   auth.MaskSecret(k.Key),
		IsActive:  k.IsActive,
		CreatedAt: k.CreatedAt,
	}
}

func toAPIKeyDTOList(keys []model.APIKey) []APIKeyDTO {
	out := make([]APIKeyDTO, 0, len(keys))
	for i := range keys {
		out = append(out, toAPIKeyDTO(&keys[i]))
	}
	return out
}

// NodeDTO is the redacted representation of a custom endpoint node: the API
// key is never returned.
type NodeDTO struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"`
	Name      string      `json:"name"`
	Data      NodeDataDTO `json:"data"`
	CreatedAt time.Time   `json:"createdAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

type NodeDataDTO struct {
	Prefix    string `json:"prefix,omitempty"`
	APIType   string `json:"apiType,omitempty"`
	BaseURL   string `json:"baseUrl,omitempty"`
	HasAPIKey bool   `json:"hasApiKey"`
}

func toNodeDTO(n *model.ProviderNode) NodeDTO {
	return NodeDTO{
		ID:        n.ID,
		Type:      n.Type,
		Name:      n.Name,
		CreatedAt: n.CreatedAt,
		UpdatedAt: n.UpdatedAt,
		Data: NodeDataDTO{
			Prefix:    n.Data.Prefix,
			APIType:   n.Data.APIType,
			BaseURL:   n.Data.BaseURL,
			HasAPIKey: n.Data.APIKey != "",
		},
	}
}
