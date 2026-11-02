package provider

// Kimi Code client identity. Mirror the official kimi-cli: Moonshot's
// kimi-for-coding backend 403s ("only available for Coding Agents") on any
// User-Agent that is not KimiCLI/<version>, and the X-Msh-* block is what the
// channel check reads for platform/version. Naming ourselves here — the old
// "cyrene-gateway"/"1.0.0" pair — identified the relay on every request.
const (
	KimiCLIVersion = "1.52.0"
	KimiCLIUA      = "KimiCLI/" + KimiCLIVersion
	// kimiCLIPlatform is the literal the official CLI sends, not a product name.
	kimiCLIPlatform = "kimi_cli"
)
