package model

import "time"

// ProviderConnection represents a provider account/connection
type ProviderConnection struct {
	ID        string         `json:"id"`
	Provider  string         `json:"provider"`
	AuthType  string         `json:"authType"`
	Name      string         `json:"name,omitempty"`
	Email     string         `json:"email,omitempty"`
	Priority  int            `json:"priority"`
	IsActive  bool           `json:"isActive"`
	Data      ConnectionData `json:"data"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

type ConnectionData struct {
	APIKey               string         `json:"apiKey,omitempty"`
	AccessToken          string         `json:"accessToken,omitempty"`
	RefreshToken         string         `json:"refreshToken,omitempty"`
	ExpiresAt            string         `json:"expiresAt,omitempty"`
	BaseURL              string         `json:"baseUrl,omitempty"`
	TestStatus           string         `json:"testStatus,omitempty"`
	LastError            string         `json:"lastError,omitempty"`
	RateLimitedUntil     string         `json:"rateLimitedUntil,omitempty"`
	BackoffLevel         int            `json:"backoffLevel,omitempty"`
	ProviderSpecificData map[string]any `json:"providerSpecificData,omitempty"`
}

// ProviderNode represents a custom compatible endpoint
type ProviderNode struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	Data      NodeData  `json:"data"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type NodeData struct {
	Prefix  string `json:"prefix,omitempty"`
	APIType string `json:"apiType,omitempty"`
	BaseURL string `json:"baseUrl,omitempty"`
	APIKey  string `json:"apiKey,omitempty"`
}

// Combo represents a model fallback combination
type Combo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind,omitempty"`
	Models    []string  `json:"models"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// APIKey represents a local API key
type APIKey struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Name      string    `json:"name,omitempty"`
	MachineID string    `json:"machineId,omitempty"`
	IsActive  bool      `json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
}

// ModelInfo represents resolved model routing info
type ModelInfo struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// UsageEntry represents a single usage record
type UsageEntry struct {
	ID               int64     `json:"id,omitempty"`
	Timestamp        time.Time `json:"timestamp"`
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	ConnectionID     string    `json:"connectionId,omitempty"`
	APIKey           string    `json:"apiKey,omitempty"`
	Endpoint         string    `json:"endpoint,omitempty"`
	PromptTokens     int       `json:"promptTokens"`
	CompletionTokens int       `json:"completionTokens"`
	Cost             float64   `json:"cost"`
	Status           string    `json:"status"`
}
