package handler

import (
	"net/http"
	"strings"
)

// Downstream client-agent taxonomy ids (stable wire values; the webui owns
// display labels). Missing UA collapses to unknown, unmatched UAs to other.
const (
	clientIDClaude     = "claude-code"
	clientIDCodex      = "codex-cli"
	clientIDOpenCode   = "opencode"
	clientIDAider      = "aider"
	clientIDCursor     = "cursor"
	clientIDCline      = "cline"
	clientIDRooCode    = "roocode"
	clientIDWindsurf   = "windsurf"
	clientIDLobeChat   = "lobechat"
	clientIDOpenWebUI  = "open-webui"
	clientIDDify       = "dify"
	clientIDCherry     = "cherry-studio"
	clientIDChatWise   = "chatwise"
	clientIDNextChat   = "nextchat"
	clientIDJan        = "jan"
	clientIDMsty       = "msty"
	clientIDLangChain  = "langchain"
	clientIDAnthropic  = "anthropic-sdk"
	clientIDOpenAI     = "openai-sdk"
	clientIDGemini     = "gemini-sdk"
	clientIDInsomnia   = "insomnia"
	clientIDPostman    = "postman"
	clientIDApifox     = "apifox"
	clientIDApiPost    = "apipost"
	clientIDHoppscotch = "hoppscotch"
	clientIDCurl       = "curl"
	clientIDWget       = "wget"
	clientIDHttpie     = "httpie"
	clientIDGoHTTP     = "go-http"
	clientIDBun        = "bun"
	clientIDNodeHTTP   = "node-http"
	clientIDPythonHTTP = "python-http"
	clientIDJavaHTTP   = "java-http"
	clientIDBrowser    = "browser"
	clientIDUnknown    = "unknown"
	clientIDOther      = "other"
)

// agentRule maps lowercase User-Agent substrings to a stable client id.
// Order matters: named agent/SDK/tool fingerprints first, generic HTTP
// libraries second, browser heuristic last — raw UAs never leak into the
// dimension and cardinality stays bounded.
type agentRule struct {
	patterns []string
	id       string
}

var agentRules = []agentRule{
	{[]string{"claude-cli", "claude_code/"}, clientIDClaude},
	{[]string{"codex_cli_rs", "codex-cli", "codex_cli"}, clientIDCodex},
	{[]string{"opencode/"}, clientIDOpenCode},
	{[]string{"aider"}, clientIDAider},
	{[]string{"cursor"}, clientIDCursor},
	{[]string{"cline"}, clientIDCline},
	{[]string{"roocode", "roo-code", "roo code"}, clientIDRooCode},
	{[]string{"windsurf"}, clientIDWindsurf},
	{[]string{"lobechat", "lobe-chat", "lobehub"}, clientIDLobeChat},
	{[]string{"open-webui"}, clientIDOpenWebUI},
	{[]string{"dify"}, clientIDDify},
	{[]string{"cherry studio", "cherry-studio"}, clientIDCherry},
	{[]string{"chatwise"}, clientIDChatWise},
	{[]string{"chatgpt-next-web", "chatgptnextweb", "nextchat"}, clientIDNextChat},
	{[]string{"jan/"}, clientIDJan},
	{[]string{"msty"}, clientIDMsty},
	{[]string{"langchain"}, clientIDLangChain},
	{[]string{"anthropic/"}, clientIDAnthropic},
	{[]string{"openai/"}, clientIDOpenAI},
	{[]string{"google-genai", "google-api-python-client"}, clientIDGemini},
	{[]string{"insomnia"}, clientIDInsomnia},
	{[]string{"postman"}, clientIDPostman},
	{[]string{"apifox"}, clientIDApifox},
	{[]string{"apipost"}, clientIDApiPost},
	{[]string{"hoppscotch"}, clientIDHoppscotch},
	{[]string{"curl/"}, clientIDCurl},
	{[]string{"wget/"}, clientIDWget},
	{[]string{"httpie"}, clientIDHttpie},
	{[]string{"go-http-client"}, clientIDGoHTTP},
	{[]string{"bun/"}, clientIDBun},
	{[]string{"node-fetch", "undici", "axios"}, clientIDNodeHTTP},
	{[]string{"python-requests", "python-urllib", "httpx", "aiohttp"}, clientIDPythonHTTP},
	{[]string{"java/", "okhttp", "apache-httpclient", "urlconnection"}, clientIDJavaHTTP},
	{[]string{"mozilla/", "applewebkit"}, clientIDBrowser},
}

// rawUserAgent returns the caller's User-Agent, capped. Paired with an "other"
// classification it is the only way to learn which client was missed -- the
// taxonomy deliberately discards the string otherwise.
func rawUserAgent(r *http.Request) string {
	const maxUA = 200
	ua := r.Header.Get("User-Agent")
	if len(ua) > maxUA {
		return ua[:maxUA] + "..."
	}
	return ua
}

// detectClientAgent fingerprints the downstream caller from its User-Agent
// header. Missing UA is "unknown"; unclassifiable UAs collapse to "other".
func detectClientAgent(r *http.Request) string {
	ua := r.Header.Get("User-Agent")
	if ua == "" {
		return clientIDUnknown
	}
	lower := strings.ToLower(ua)
	for _, rule := range agentRules {
		for _, p := range rule.patterns {
			if strings.Contains(lower, p) {
				return rule.id
			}
		}
	}
	return clientIDOther
}
