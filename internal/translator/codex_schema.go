package translator

import (
	"regexp"
)

// unicodePropertyEscape matches \p{...} or \P{...} with an odd number of preceding backslashes.
// An even number of backslashes means the backslash itself is escaped (\\p{...} is literal p).
var unicodePropertyEscape = regexp.MustCompile(`(?:^|[^\\])(?:\\\\)*\\[pP]\{`)

func hasUnicodePropertyEscape(pattern string) bool {
	return unicodePropertyEscape.MatchString(pattern)
}

// StripCodexUnsupportedPatterns recursively strips "pattern" constraints from tool parameter
// schemas if the pattern contains Unicode property escapes that Codex's validator rejects (9router#3922).
func StripCodexUnsupportedPatterns(schema map[string]any) {
	if schema == nil {
		return
	}
	stripCodexPatternsWalk(schema, true)
}

func stripCodexPatternsWalk(obj map[string]any, isSchema bool) {
	if isSchema {
		if pat, ok := obj["pattern"].(string); ok {
			if hasUnicodePropertyEscape(pat) {
				delete(obj, "pattern")
			}
		}
	}
	walkChildren(obj, isSchema, stripCodexPatternsWalk)
}
