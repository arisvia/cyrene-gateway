package handler

import (
	"net/http"
	"strings"
)

// Downstream client identity, derived from the User-Agent instead of matched
// against a catalog of known tools.
//
// A hardcoded rule list is always one release behind: every new CLI, every
// renamed product, and every SDK minor bump silently collapses to "other",
// which is exactly the dimension most worth reading. User-Agents already carry
// the product name and version in a conventional "Product/Version" shape, so
// the gateway parses that shape and reports what it finds. Unknown tools then
// show up under their real name instead of a bucket that explains nothing.
//
// Wire values: the product token, lowercased, with version reported separately.
// Missing UA is still "unknown"; a UA with no product token is "other".
const (
	clientIDUnknown = "unknown"
	clientIDOther   = "other"

	// Browsers are the one shape that does not lead with the product: a
	// Mozilla-prefixed UA is a whole platform string, and its later tokens
	// (AppleWebKit, Safari) are engine names, not callers. Reporting the raw
	// token would say "mozilla" for every browser visit.
	clientIDBrowser = "browser"

	maxClientName    = 64
	maxClientVersion = 32
)

// parseUserAgent extracts the calling product and its version.
//
// Handles the two shapes that actually occur:
//
//	"claude-cli/2.0.59 (external, cli)"  -> claude-cli, 2.0.59
//	"Aider 0.74.1"                       -> aider, 0.74.1
//
// The product is the first token; the version is the numeric segment that
// follows it, either after a slash or somewhere later in the same field. A UA
// that carries no version simply reports none -- nothing is invented.
func parseUserAgent(ua string) (name string, version string) {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return "", ""
	}
	if strings.HasPrefix(strings.ToLower(ua), "mozilla/") {
		return clientIDBrowser, ""
	}

	fields := strings.Fields(ua)
	if len(fields) == 0 {
		return "", ""
	}
	head := fields[0]

	// A leading slash means no product token at all. Without this guard the
	// slash survives to sanitizeToken as "/1.2.3" and trims down to a version
	// pretending to be a client name.
	if strings.HasPrefix(head, "/") {
		return "", ""
	}
	product, rest := head, ""
	if i := strings.IndexByte(head, '/'); i > 0 {
		product, rest = head[:i], head[i+1:]
	}
	product = sanitizeToken(product)
	if product == "" {
		return "", ""
	}

	if rest != "" && looksLikeVersion(rest) {
		version = sanitizeToken(rest)
	}
	// The version may also follow as later tokens: "Anthropic/Python 0.42.0"
	// puts a runtime name between the product and the number, and "Aider 0.74.1"
	// has no slash at all. Scanning for the first numeric-looking token is the
	// same structural rule used everywhere else -- no product catalog needed.
	if version == "" {
		for _, field := range fields[1:] {
			tok := sanitizeToken(field)
			if looksLikeVersion(tok) {
				version = tok
				break
			}
		}
	}

	return product, truncate(version, maxClientVersion)
}

// sanitizeToken lowercases a UA token and trims the punctuation that separates
// it from the next field, keeping the result safe as a grouping key.
func sanitizeToken(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Trim(s, "()[]{};,\"/")
	if s == "" || s == "/" {
		return ""
	}
	// A trailing parenthetical ("opencode/1.4.0(x)") leaves a dangling comma or
	// space once trimmed; cut the version at the first character that cannot be
	// part of one.
	if i := strings.IndexAny(s, " \t(),;"); i >= 0 {
		s = s[:i]
	}
	return truncate(s, maxClientName)
}

func looksLikeVersion(s string) bool {
	if s == "" {
		return false
	}
	return s[0] >= '0' && s[0] <= '9'
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// detectClientAgent reports the calling product, e.g. "claude-cli".
func detectClientAgent(r *http.Request) string {
	name, _ := parseUserAgent(r.Header.Get("User-Agent"))
	if name == "" {
		if strings.TrimSpace(r.Header.Get("User-Agent")) == "" {
			return clientIDUnknown
		}
		return clientIDOther
	}
	return name
}

// detectClientVersion reports the version that followed the product token.
// Empty when the UA carried none, which is common for simple clients.
func detectClientVersion(r *http.Request) string {
	_, version := parseUserAgent(r.Header.Get("User-Agent"))
	return version
}

// rawUserAgent returns the caller's User-Agent, capped. Kept alongside the
// parsed identity because parsing is a lossy convenience, not a source of truth.
func rawUserAgent(r *http.Request) string {
	const maxUA = 200
	ua := r.Header.Get("User-Agent")
	if len(ua) > maxUA {
		return ua[:maxUA] + "..."
	}
	return ua
}
