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

// AntigravityAPIProviderInternal is the apiProvider enum upstream stamps on its own
// test endpoints, alongside isInternal.
const AntigravityAPIProviderInternal = "API_PROVIDER_INTERNAL"

// AntigravityInternalEngine reports whether an advertised catalog entry is not a
// servable chat model. Upstream states this itself: isInternal and the INTERNAL api
// surface mark test endpoints, and requiresLeadInGeneration marks the editor's
// inline-completion anchors, which carry quotaInfo but no chat surface. Reading those
// flags replaces guessing from id prefixes (chat_, tab_, rev_), which upstream is free
// to rename — verified live: both chat_* entries are isInternal AND INTERNAL, while
// the two tab_* anchors are neither, and only requiresLeadInGeneration identifies them.
func AntigravityInternalEngine(isInternal bool, apiProvider string, requiresLeadInGeneration bool) bool {
	return isInternal || requiresLeadInGeneration || apiProvider == AntigravityAPIProviderInternal
}
