package provider

import (
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// Auth scheme constants for upstream credential injection. These mirror the
// 9router registry transport.auth.scheme values (open-sse/executors/default.js).
const (
	// AuthBearer sends "Authorization: Bearer <token>".
	AuthBearer = "bearer"
	// AuthRaw sends the token verbatim in the configured header (e.g. x-api-key).
	AuthRaw = "raw"
	// AuthQuery sends the token as a URL query parameter (e.g. Gemini ?key=).
	AuthQuery = "query"
	// AuthCookie sends the token as a Cookie header (webCookie providers).
	AuthCookie = "cookie"
)

// anthropicAPIVersion is the single source for the Anthropic API version header,
// reused across all claude-format providers (9router shared.js ANTHROPIC_API_VERSION).
const anthropicAPIVersion = "2023-06-01"

// Transport describes the runtime HTTP configuration for reaching a provider's
// upstream API: where to send the request, what wire format to use, and how to
// authenticate. It is resolved per-request from the registry plus the active
// connection (ResolveTransport), ported from 9router registry transport blocks.
type Transport struct {
	// BaseURL is the upstream endpoint base (may already be a full endpoint).
	BaseURL string
	// Format is the wire format: "openai", "anthropic", or "gemini".
	Format string
	// Headers are extra headers sent on every request (e.g. anthropic-version).
	Headers map[string]string
	// URLSuffix is appended verbatim to the base URL (e.g. "?beta=true").
	URLSuffix string
	// Auth carries the credential injection descriptor.
	Auth AuthDescriptor
}

// AuthDescriptor describes how a credential token is attached to an upstream
// request. Ported from 9router transport.auth + AUTH_DESCRIPTORS.
type AuthDescriptor struct {
	// Header is the header name carrying the token (ignored for AuthQuery).
	Header string
	// Scheme is one of AuthBearer, AuthRaw, AuthQuery.
	Scheme string
	// QueryParam is the query parameter name when Scheme == AuthQuery.
	QueryParam string
	// AnthropicVersion injects the anthropic-version header when set.
	AnthropicVersion bool
	// Hooks are provider-specific header overlays run BEFORE auth is applied
	// (e.g. kimiHeaders), so dynamic overlays cannot clobber the token.
	Hooks []string
}

// Credentials is the subset of connection data the transport layer needs to
// authenticate an upstream request.
type Credentials struct {
	APIKey      string
	AccessToken string
	// ProviderSpecificData carries per-connection metadata (e.g. kimi deviceId).
	ProviderSpecificData map[string]any
}

// credentialsFromConn projects a connection onto the transport Credentials view.
func credentialsFromConn(conn *model.ProviderConnection) Credentials {
	return Credentials{
		APIKey:               conn.Data.APIKey,
		AccessToken:          conn.Data.AccessToken,
		ProviderSpecificData: conn.Data.ProviderSpecificData,
	}
}

// token returns the credential value to inject, preferring an API key over an
// OAuth access token (matches 9router applyAuth: apiKey || accessToken).
func (c Credentials) token() string {
	if c.APIKey != "" {
		return c.APIKey
	}
	return c.AccessToken
}

// ResolveTransport determines the effective transport for a request. baseURL and
// apiType must be the already-resolved effective values (auth-mode override per
// 9router#2881, plus any user connection override) so transport always agrees
// with the format the caller selected for translation.
func ResolveTransport(p ProviderInfo, baseURL, apiType string, conn *model.ProviderConnection) Transport {
	creds := credentialsFromConn(conn)
	t := Transport{
		BaseURL: baseURL,
		Format:  apiType,
		Headers: p.Headers,
		// Default: derive auth from the effective format + credential type.
		Auth: deriveAuthDescriptor(apiType, creds),
	}

	// Explicit registry transport config applies only when the request follows
	// the provider's primary path (no auth-mode base URL override, no user base
	// URL). This is what makes dual-auth work: kimi's OAuth path keeps its
	// declared x-api-key/kimiHeaders transport, while its apikey path (routed to
	// the moonshot OpenAI endpoint via ApiKeyBaseURL) derives plain Bearer auth.
	if conn != nil && conn.Data.BaseURL == "" && !(creds.APIKey != "" && p.ApiKeyBaseURL != "") {
		if p.URLSuffix != "" {
			t.URLSuffix = p.URLSuffix
		}
		if p.AuthScheme != "" {
			t.Auth = AuthDescriptor{
				Header: p.AuthHeader,
				Scheme: p.AuthScheme,
				Hooks:  p.AuthHooks,
			}
		} else if len(p.AuthHooks) > 0 {
			// Hooks-only transport (e.g. kimi apikey path still wants X-Msh-*).
			t.Auth.Hooks = append(t.Auth.Hooks, p.AuthHooks...)
		}
	}

	return t
}

// deriveAuthDescriptor produces the default auth descriptor for a format when the
// registry entry declares no explicit auth. It preserves the pre-Phase-30 behavior:
// api-key credentials follow the format convention (x-api-key for claude, ?key for
// gemini, Bearer otherwise); OAuth access tokens default to Bearer.
func deriveAuthDescriptor(apiType string, creds Credentials) AuthDescriptor {
	if creds.APIKey != "" {
		switch apiType {
		case "anthropic":
			return AuthDescriptor{Header: "x-api-key", Scheme: AuthRaw, AnthropicVersion: true}
		case "gemini":
			return AuthDescriptor{Scheme: AuthQuery, QueryParam: "key"}
		}
	}
	return AuthDescriptor{Header: "Authorization", Scheme: AuthBearer}
}

// BuildTransportURL constructs the upstream request URL for a transport, handling
// urlSuffix, gemini model/verb paths, and already-full endpoint URLs.
func BuildTransportURL(t Transport, modelName string, stream bool) string {
	base := strings.TrimRight(t.BaseURL, "/")
	if base == "" {
		return ""
	}
	if t.URLSuffix != "" {
		return base + t.URLSuffix
	}
	if t.Format == "gemini" {
		return BuildGeminiURL(base, modelName, stream)
	}
	return BuildChatURL(base, t.Format)
}

// ApplyAuth attaches authentication to an upstream request per the transport's
// descriptor. Hooks run first so dynamic overlays cannot clobber the token.
func ApplyAuth(req *http.Request, t Transport, creds Credentials) {
	for _, hook := range t.Auth.Hooks {
		if fn, ok := authHooks[hook]; ok {
			fn(req.Header, creds)
		}
	}

	token := creds.token()
	if token == "" {
		return
	}

	switch t.Auth.Scheme {
	case AuthQuery:
		param := t.Auth.QueryParam
		if param == "" {
			param = "key"
		}
		q := req.URL.Query()
		q.Set(param, token)
		req.URL.RawQuery = q.Encode()
	case AuthRaw:
		if t.Auth.Header != "" {
			req.Header.Set(t.Auth.Header, token)
		}
		if t.Auth.AnthropicVersion && req.Header.Get("anthropic-version") == "" {
			req.Header.Set("anthropic-version", anthropicAPIVersion)
		}
	case AuthCookie:
		req.Header.Set("Cookie", token)
	default: // AuthBearer
		header := t.Auth.Header
		if header == "" {
			header = "Authorization"
		}
		req.Header.Set(header, "Bearer "+token)
	}
}

// authHooks is the registry of provider-specific header overlays. Ported from
// 9router HEADER_HOOKS (open-sse/executors/default.js).
var authHooks = map[string]func(http.Header, Credentials){
	"kimiHeaders": kimiHeadersHook,
}

// kimiHeadersHook injects the X-Msh-* client fingerprint headers required by the
// Kimi coding endpoint (CLIProxyAPI KimiTokenStorage parity). The device ID is
// stable per connection when stored in providerSpecificData.deviceId.
func kimiHeadersHook(h http.Header, c Credentials) {
	deviceID := ""
	if c.ProviderSpecificData != nil {
		if id, ok := c.ProviderSpecificData["deviceId"].(string); ok {
			deviceID = strings.TrimSpace(id)
		}
	}
	if deviceID == "" {
		deviceID = "kimi-" + time.Now().Format("20060102150405.000")
	}
	h.Set("X-Msh-Platform", "cyrene-gateway")
	h.Set("X-Msh-Version", "1.0.0")
	h.Set("X-Msh-Device-Name", deviceName())
	h.Set("X-Msh-Device-Model", deviceModel())
	h.Set("X-Msh-Device-Id", deviceID)
}

func deviceModel() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS " + runtime.GOARCH
	case "windows":
		return "Windows " + runtime.GOARCH
	case "linux":
		return "Linux " + runtime.GOARCH
	default:
		return runtime.GOOS + " " + runtime.GOARCH
	}
}

func deviceName() string {
	if hn, err := os.Hostname(); err == nil && hn != "" {
		return hn
	}
	return "unknown"
}
