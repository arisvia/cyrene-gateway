package provider

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// TestDialerControlExemptSemantics pins the permit-list behavior of the
// SSRF dialer control: proxy endpoints (often 127.0.0.1 for local pools)
// are dialable, while every other private/loopback/metadata address stays blocked.
func TestDialerControlExemptSemantics(t *testing.T) {
	plain := SafeDialerControl(false)
	if err := plain("tcp", "127.0.0.1:8080", nil); err == nil {
		t.Error("plain control must block loopback")
	}

	exempt := dialerControlExempt(false, map[string]bool{"127.0.0.1": true})
	if err := exempt("tcp", "127.0.0.1:8080", nil); err != nil {
		t.Errorf("exempted proxy host must be dialable, got: %v", err)
	}
	if err := exempt("tcp", "169.254.169.254:80", nil); err == nil {
		t.Error("metadata address must stay blocked even with a different host exempted")
	}

	if err := dialerControlExempt(true, nil)("tcp", "127.0.0.1:8080", nil); err != nil {
		t.Errorf("allowPrivate must bypass blocking, got: %v", err)
	}
}

// TestProxyClientKeepsSSRFGuardOnRedirect proves the proxy-rotation client
// still validates redirect targets through CheckRedirect: a redirect to the
// cloud metadata address must be rejected even though the dial goes through
// a loopback proxy (the standard Clash/V2Ray shape).
func TestProxyClientKeepsSSRFGuardOnRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer target.Close()

	pm := NewProxyManager(nil)
	client := pm.GetHTTPClient(false)
	if client.Timeout <= 0 {
		t.Fatalf("proxy client must carry a bounded default timeout, got %v", client.Timeout)
	}

	proxyURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	exempt := map[string]bool{proxyURL.Hostname(): true}
	transport := &http.Transport{
		DialContext: safeDialer(false, exempt).DialContext,
		Proxy:       http.ProxyURL(proxyURL),
	}
	client.Transport = transport
	client.Timeout = 10 * time.Second

	resp, err := client.Get("http://example.invalid/")
	if err == nil {
		resp.Body.Close()
		t.Fatal("redirect to metadata address must fail")
	}
	// Chain-based: only the CheckRedirect path can yield this sentinel here
	// (initial URL never passes through ValidateUpstreamURL; the proxy endpoint
	// is exempt from dial-time blocking).
	if !errors.Is(err, ErrPrivateNetworkBlocked) {
		t.Fatalf("expected ErrPrivateNetworkBlocked via redirect guard, got: %v", err)
	}
	// ssrf.go formats the blocked target verbatim: "…resolved to 169.254.169.254".
	if !strings.Contains(err.Error(), "169.254.169.254") {
		t.Fatalf("guard must fire on the redirect TARGET (%v), not the proxy dial; got: %v", err, proxyURL)
	}
}

// TestGetHTTPClientWithoutProxiesIsSafe checks the no-proxy fallback keeps the
// hardened transport and a non-zero timeout (no unbounded http.Client{}).
func TestGetHTTPClientWithoutProxiesIsSafe(t *testing.T) {
	client := NewProxyManager(nil).GetHTTPClient(false)
	if client.Timeout <= 0 {
		t.Fatalf("expected bounded timeout, got %v", client.Timeout)
	}
	tr, ok := client.Transport.(*http.Transport)
	if !ok || tr.DialContext == nil {
		t.Fatal("expected SSRF-safe transport with DialContext")
	}
}

// TestProxyExemptHosts pins the permit-list construction: literal-IP proxy
// spellings resolve to themselves, and unresolvable hostnames fail closed to
// the hostname-only set.
func TestProxyExemptHosts(t *testing.T) {
	u, err := url.Parse("http://127.0.0.1:7890")
	if err != nil {
		t.Fatal(err)
	}
	ex := proxyExemptHosts(u)
	if !ex["127.0.0.1"] {
		t.Errorf("literal IP must be exempt verbatim, got %v", ex)
	}

	bad, _ := url.Parse("http://this-host-does-not-exist.invalid:7890")
	ex2 := proxyExemptHosts(bad)
	if len(ex2) != 1 || !ex2["this-host-does-not-exist.invalid"] {
		t.Errorf("unresolvable host must fail closed to hostname-only set, got %v", ex2)
	}
}

// TestActivePoolE2EBlocksMetadataRedirect is the composed end-to-end test of the
// production proxy branch: a REAL active ProxyManager pool driven through
// GetHTTPClient(false) — no hand-built transport. The pool proxy (a loopback
// httptest server, i.e. the Clash/V2Ray shape) answers with a 302 to the cloud
// metadata address; the client's CheckRedirect must reject that target.
func TestActivePoolE2EBlocksMetadataRedirect(t *testing.T) {
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer proxySrv.Close()

	pm := NewProxyManager([]model.ProxyPool{{
		ID:       "pool-e2e",
		IsActive: true,
		Data:     model.ProxyPoolData{Name: "local", ProxyURL: proxySrv.URL},
	}})
	if !pm.HasProxies() {
		t.Fatal("pool must be active")
	}

	client := pm.GetHTTPClient(false)
	client.Timeout = 10 * time.Second

	resp, err := client.Get("http://any-target.example/")
	if err == nil {
		resp.Body.Close()
		t.Fatal("metadata redirect through active proxy pool must be blocked")
	}
	if !errors.Is(err, ErrPrivateNetworkBlocked) {
		t.Fatalf("expected ErrPrivateNetworkBlocked via redirect guard, got: %v", err)
	}
	if !strings.Contains(err.Error(), "169.254.169.254") {
		t.Fatalf("guard must fire on the redirect TARGET, not the proxy dial; got: %v", err)
	}
}
