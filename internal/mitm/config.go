package mitm

// ToolHosts maps tool names to the domains they use.
var ToolHosts = map[string][]string{
	"antigravity": {"daily-cloudcode-pa.googleapis.com", "cloudcode-pa.googleapis.com"},
	"copilot":     {"api.individual.githubcopilot.com"},
	"kiro":        {"runtime.us-east-1.kiro.dev", "q.us-east-1.amazonaws.com", "codewhisperer.us-east-1.amazonaws.com"},
	"cursor":      {"api2.cursor.sh"},
}

// URLPatterns maps tool names to URL substrings that identify chat requests.
var URLPatterns = map[string][]string{
	"antigravity": {":generateContent", ":streamGenerateContent"},
	"copilot":     {"/chat/completions", "/v1/messages", "/responses"},
	"kiro":        {"/generateAssistantResponse"},
	"cursor":      {"/BidiAppend", "/RunSSE", "/RunPoll", "/Run"},
}

// HostRewrite maps upstream hosts to alternative hosts for rate-limit avoidance.
var HostRewrite = map[string]string{
	"cloudcode-pa.googleapis.com": "daily-cloudcode-pa.googleapis.com",
}

// GetToolForHost returns the tool name for a given host (without port).
func GetToolForHost(host string) string {
	// Strip port if present
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			host = host[:i]
			break
		}
		if host[i] == ']' {
			break // IPv6 bracket, no port
		}
	}

	switch host {
	case "api.individual.githubcopilot.com":
		return "copilot"
	case "daily-cloudcode-pa.googleapis.com", "cloudcode-pa.googleapis.com":
		return "antigravity"
	case "q.us-east-1.amazonaws.com", "codewhisperer.us-east-1.amazonaws.com", "runtime.us-east-1.kiro.dev":
		return "kiro"
	case "api2.cursor.sh":
		return "cursor"
	}
	return ""
}

// AllToolHosts returns a flat list of all intercepted hosts.
func AllToolHosts() []string {
	var hosts []string
	for _, h := range ToolHosts {
		hosts = append(hosts, h...)
	}
	return hosts
}
