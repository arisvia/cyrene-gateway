package provider

// Antigravity protocol constants shared across handler, discovery, and usage.
const (
	AntigravityBaseURL  = "https://daily-cloudcode-pa.googleapis.com"
	AntigravityDailyURL = "https://daily-cloudcode-pa.sandbox.googleapis.com"
	// Version-gated: Google returns 404 (not "model missing") for tiers this
	// fingerprint predates — Gemini 4 Argon needed 2.24.0. Bump only when a new
	// model 404s here, not to chase the latest App build: a newer UA can bring
	// new required headers and 403 on the older call shape.
	AntigravityUserAgent = "antigravity/hub/2.24.0 darwin/arm64"
	AntigravityMetadata  = `{"ideType":"ANTIGRAVITY","platform":"PLATFORM_UNSPECIFIED","pluginType":"GEMINI"}`
)
