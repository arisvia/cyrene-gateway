package model

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

// Query-string auth (gemini catalogs) puts the API key in the URL, and a
// transport error carries the whole URL into the response body and the log.
func TestRedactURLErrorStripsCredentials(t *testing.T) {
	full := "https://generativelanguage.googleapis.com/v1beta/models?key=AIzaSECRET"
	err := &url.Error{Op: "Get", URL: full, Err: errors.New("dial tcp: timeout")}

	out := redactURLError(err)
	if strings.Contains(out, "AIzaSECRET") {
		t.Fatalf("secret survived redaction: %s", out)
	}
	if !strings.Contains(out, "timeout") {
		t.Fatalf("redaction dropped the useful part: %s", out)
	}

	// Non-URL errors pass through unchanged.
	if got := redactURLError(errors.New("plain failure")); got != "plain failure" {
		t.Fatalf("unexpected rewrite: %s", got)
	}
}

// A nil client must fail closed rather than silently constructing a client
// without SSRF protection.
func TestLoadModelsDevCatalogRejectsNilClient(t *testing.T) {
	if _, err := LoadModelsDevCatalog(nil); err == nil {
		t.Fatal("nil client must be rejected, not replaced with an unguarded one")
	}
}
