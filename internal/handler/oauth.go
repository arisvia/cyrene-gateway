package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// handleOAuthAuthorize generates an authorization URL for a provider.
// GET /api/oauth/{provider}/authorize?redirect_uri=...
func (s *Server) handleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("provider")
	if providerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing provider"})
		return
	}

	if _, ok := provider.GetProvider(providerID); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown provider: " + providerID})
		return
	}

	flowType := provider.GetProviderFlowType(providerID)
	if flowType == provider.FlowDeviceCode {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider uses device code flow, use /api/oauth/{provider}/device-code"})
		return
	}
	if flowType == provider.FlowImportToken {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider uses token import, use /api/oauth/{provider}/import"})
		return
	}

	redirectURI := r.URL.Query().Get("redirect_uri")
	if redirectURI == "" {
		redirectURI = "http://localhost:8080/callback"
	}

	pkce, err := provider.GeneratePKCE()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate PKCE"})
		return
	}

	authURL, err := provider.BuildAuthorizeURL(providerID, redirectURI, pkce)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Store session for callback validation
	provider.StoreSession(pkce.State, &provider.OAuthSession{
		Provider:    providerID,
		PKCE:        pkce,
		RedirectURI: redirectURI,
		CreatedAt:   time.Now(),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"authorizeUrl": authURL,
		"state":        pkce.State,
		"codeVerifier": pkce.CodeVerifier,
		"redirectUri":  redirectURI,
		"flowType":     string(flowType),
	})
}

// handleOAuthCallback exchanges an authorization code for tokens and creates a connection.
// GET /api/oauth/{provider}/callback?code=...&state=...
func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("provider")
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing authorization code"})
		return
	}

	// State is mandatory for OAuth callback to prevent CSRF / code injection
	if state == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing state parameter"})
		return
	}
	session, ok := provider.GetSession(state)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired state"})
		return
	}
	if session.Provider != providerID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "state provider mismatch"})
		return
	}
	codeVerifier := session.PKCE.CodeVerifier
	redirectURI := session.RedirectURI
	provider.ClearSession(state)

	// Exchange code for tokens
	tokens, err := provider.ExchangeCode(providerID, code, redirectURI, codeVerifier, nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	// Create connection
	conn := s.createOAuthConnection(providerID, tokens)
	if conn == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create connection"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"connection": map[string]any{
			"id":          conn.ID,
			"provider":    conn.Provider,
			"email":       conn.Email,
			"displayName": conn.Name,
		},
	})
}

// handleOAuthExchange exchanges an authorization code for tokens (POST variant).
// POST /api/oauth/{provider}/exchange
func (s *Server) handleOAuthExchange(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("provider")

	var req struct {
		Code         string `json:"code"`
		RedirectURI  string `json:"redirectUri"`
		CodeVerifier string `json:"codeVerifier"`
		State        string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing code"})
		return
	}

	// Detect raw JWT access token (starts with eyJ)
	if len(req.Code) > 3 && req.Code[:3] == "eyJ" {
		tokens, err := provider.ImportToken(providerID, req.Code)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		conn := s.createOAuthConnection(providerID, tokens)
		if conn == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create connection"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"connection": map[string]any{
				"id":       conn.ID,
				"provider": conn.Provider,
				"email":    conn.Email,
			},
		})
		return
	}

	// Use session data if state provided and no explicit verifier
	// If state provided without explicit codeVerifier, session must exist
	if req.State != "" && req.CodeVerifier == "" {
		session, ok := provider.GetSession(req.State)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired state"})
			return
		}
		if session.Provider != providerID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "state provider mismatch"})
			return
		}
		req.CodeVerifier = session.PKCE.CodeVerifier
		if req.RedirectURI == "" {
			req.RedirectURI = session.RedirectURI
		}
		provider.ClearSession(req.State)
	}
	tokens, err := provider.ExchangeCode(providerID, req.Code, req.RedirectURI, req.CodeVerifier, nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	conn := s.createOAuthConnection(providerID, tokens)
	if conn == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create connection"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"connection": map[string]any{
			"id":          conn.ID,
			"provider":    conn.Provider,
			"email":       conn.Email,
			"displayName": conn.Name,
		},
	})
}

// handleOAuthDeviceCode initiates a device code flow.
// POST /api/oauth/{provider}/device-code
func (s *Server) handleOAuthDeviceCode(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("provider")

	if _, ok := provider.GetProvider(providerID); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown provider: " + providerID})
		return
	}

	// Qoder uses a custom device-token flow (local PKCE + browser login + GET poll)
	if providerID == "qoder" {
		flow := provider.InitiateQoderDeviceFlow()
		writeJSON(w, http.StatusOK, map[string]any{
			"verificationUri":         flow.VerificationURI,
			"verificationUriComplete": flow.VerificationURI,
			"codeVerifier":            flow.CodeVerifier,
			"nonce":                   flow.Nonce,
			"machineId":               flow.MachineID,
			"expiresIn":               300,
			"interval":                2,
		})
		return
	}

	flowType := provider.GetProviderFlowType(providerID)
	if flowType != provider.FlowDeviceCode {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider does not support device code flow"})
		return
	}

	result, err := provider.RequestDeviceCode(providerID, nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleOAuthDeviceCodePoll polls for a device code token.
// POST /api/oauth/{provider}/device-code/poll
func (s *Server) handleOAuthDeviceCodePoll(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("provider")

	var req struct {
		ExtraData    map[string]any `json:"extraData"`
		DeviceCode   string         `json:"deviceCode"`
		CodeVerifier string         `json:"codeVerifier"`
		Nonce        string         `json:"nonce"`
		MachineID    string         `json:"machineId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	// Qoder custom poll: GET with nonce + verifier
	if providerID == "qoder" {
		result, err := provider.PollQoderDeviceToken(req.Nonce, req.CodeVerifier, nil)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		if result.Status == "ok" {
			name, email := provider.FetchQoderUserInfo(result.AccessToken, nil)
			expiresAt := ""
			if result.ExpiresAt > 0 {
				expiresAt = time.UnixMilli(result.ExpiresAt).UTC().Format(time.RFC3339)
			}
			conn := &model.ProviderConnection{
				ID:       generateID(),
				Provider: providerID,
				AuthType: "oauth",
				Name:     name,
				Email:    email,
				Priority: 0,
				IsActive: true,
				Data: model.ConnectionData{
					AccessToken:  result.AccessToken,
					RefreshToken: result.RefreshToken,
					ExpiresAt:    expiresAt,
					TestStatus:   "active",
					ProviderSpecificData: map[string]any{
						"userId":    result.UserID,
						"machineId": req.MachineID,
					},
				},
			}
			if conn.Name == "" {
				conn.Name = email
			}
			if err := s.DB.CreateConnection(conn); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create connection"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"success": true,
				"connection": map[string]any{
					"id":       conn.ID,
					"provider": conn.Provider,
					"email":    conn.Email,
				},
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"pending": true,
		})
		return
	}

	if req.DeviceCode == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing device code"})
		return
	}

	result, err := provider.PollDeviceCode(providerID, req.DeviceCode, req.CodeVerifier, req.ExtraData, nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	if result.Success && result.Tokens != nil {
		conn := s.createOAuthConnection(providerID, result.Tokens)
		if conn == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create connection"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"connection": map[string]any{
				"id":       conn.ID,
				"provider": conn.Provider,
				"email":    conn.Email,
			},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": false,
		"pending": result.Pending,
		"error":   result.Error,
	})
}

// handleOAuthImport imports a manually pasted token.
// POST /api/oauth/{provider}/import
func (s *Server) handleOAuthImport(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("provider")

	if _, ok := provider.GetProvider(providerID); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown provider: " + providerID})
		return
	}

	var req struct {
		AccessToken string `json:"accessToken"`
		Name        string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.AccessToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "access token is required"})
		return
	}

	tokens, err := provider.ImportToken(providerID, req.AccessToken)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	conn := s.createOAuthConnection(providerID, tokens)
	if conn == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create connection"})
		return
	}

	if req.Name != "" {
		conn.Name = req.Name
		s.DB.UpdateConnection(conn)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"connection": map[string]any{
			"id":       conn.ID,
			"provider": conn.Provider,
			"email":    conn.Email,
			"name":     conn.Name,
		},
	})
}

// handleOAuthStatus returns the OAuth connection status for a provider.
// GET /api/oauth/{provider}/status
func (s *Server) handleOAuthStatus(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("provider")

	conns, err := s.DB.ListConnectionsByProvider(providerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list connections"})
		return
	}

	type connStatus struct {
		ID        string `json:"id"`
		Email     string `json:"email,omitempty"`
		Name      string `json:"name,omitempty"`
		AuthType  string `json:"authType"`
		ExpiresAt string `json:"expiresAt,omitempty"`
		IsActive  bool   `json:"isActive"`
		Expired   bool   `json:"expired"`
	}

	var statuses []connStatus
	for _, c := range conns {
		if c.AuthType != "oauth" && c.AuthType != "access_token" {
			continue
		}
		expired := false
		if c.Data.ExpiresAt != "" {
			if t, err := time.Parse(time.RFC3339, c.Data.ExpiresAt); err == nil {
				expired = time.Now().After(t)
			}
		}
		statuses = append(statuses, connStatus{
			ID:        c.ID,
			Email:     c.Email,
			Name:      c.Name,
			AuthType:  c.AuthType,
			IsActive:  c.IsActive,
			ExpiresAt: c.Data.ExpiresAt,
			Expired:   expired,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"provider":    providerID,
		"flowType":    string(provider.GetProviderFlowType(providerID)),
		"connections": statuses,
	})
}

// handleOAuthRefresh manually triggers a token refresh for a connection.
// POST /api/oauth/{provider}/refresh
func (s *Server) handleOAuthRefresh(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("provider")

	var req struct {
		ConnectionID string `json:"connectionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.ConnectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connectionId is required"})
		return
	}

	conn, err := s.DB.GetConnection(req.ConnectionID)
	if err != nil || conn == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}
	if conn.Provider != providerID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection provider mismatch"})
		return
	}
	if conn.Data.RefreshToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no refresh token available"})
		return
	}

	result, refreshErr := provider.DedupRefresh(providerID, conn.Data.AccessToken, func() (*provider.RefreshResult, error) {
		return provider.RefreshCredentials(providerID, conn, nil)
	})
	if refreshErr != nil {
		status := http.StatusBadGateway
		if provider.IsUnrecoverableRefreshError(refreshErr) {
			status = http.StatusGone
			// Mark connection as needing re-auth.
			conn.Data.TestStatus = "expired"
			conn.Data.LastError = refreshErr.Error()
			s.DB.UpdateConnection(conn)
		}
		writeJSON(w, status, map[string]string{"error": refreshErr.Error()})
		return
	}

	provider.ApplyRefreshResult(conn, result)
	s.DB.UpdateConnection(conn)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"expiresAt": conn.Data.ExpiresAt,
	})
}

// createOAuthConnection creates a provider connection from OAuth token exchange results.
func (s *Server) createOAuthConnection(providerID string, tokens *provider.TokenExchangeResult) *model.ProviderConnection {
	authType := "oauth"
	if tokens.RefreshToken == "" && tokens.ProviderSpecificData != nil {
		if method, ok := tokens.ProviderSpecificData["authMethod"].(string); ok && method == "access_token" {
			authType = "access_token"
		}
	}

	expiresAt := ""
	if tokens.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}

	conn := &model.ProviderConnection{
		ID:       generateID(),
		Provider: providerID,
		AuthType: authType,
		Name:     tokens.DisplayName,
		Email:    tokens.Email,
		Priority: 0,
		IsActive: true,
		Data: model.ConnectionData{
			AccessToken:          tokens.AccessToken,
			RefreshToken:         tokens.RefreshToken,
			ExpiresAt:            expiresAt,
			TestStatus:           "active",
			ProviderSpecificData: tokens.ProviderSpecificData,
		},
	}

	if conn.Name == "" && conn.Email != "" {
		conn.Name = conn.Email
	}

	if err := s.DB.CreateConnection(conn); err != nil {
		return nil
	}
	return conn
}
