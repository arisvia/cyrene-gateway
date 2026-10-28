package provider

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func pool(url string, active, strict bool) model.ProxyPool {
	p := model.ProxyPool{ID: url, IsActive: active}
	p.Data.Name = "p"
	p.Data.ProxyURL = url
	p.Data.StrictProxy = strict
	return p
}

// A dead proxy route must never fall back to direct: requests fail at the
// transport with the strict refusal, not by dialing upstream bare.
func TestStrictPoolRefusesDirectWhenNoRouteSelectable(t *testing.T) {
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("strict mode dialed upstream directly")
	}))
	defer direct.Close()

	pm := NewProxyManager([]model.ProxyPool{pool("socks5://127.0.0.1:1", false, true)})
	if pm.HasProxies() {
		t.Fatal("inactive pool must not count as selectable")
	}
	_, err := pm.GetHTTPClient(false).Get(direct.URL)
	if err == nil || !strings.Contains(err.Error(), "refusing direct connection") {
		t.Fatalf("expected strict refusal, got %v", err)
	}
}

// Without any strict pool, an empty pool set keeps the legacy default route
// (direct/env proxy) — behavior unchanged.
func TestNonStrictEmptyPoolsStillRouteDirect(t *testing.T) {
	got := "none"
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = "hit"
	}))
	defer direct.Close()

	pm := NewProxyManager([]model.ProxyPool{pool("http://127.0.0.1:1", false, false)})
	if _, err := pm.GetHTTPClient(true).Get(direct.URL); err != nil {
		t.Fatalf("non-strict default route failed: %v", err)
	}
	if got != "hit" {
		t.Fatalf("direct route not used: %s", got)
	}
}

// The strict intent survives deactivation of the last strict pool and is only
// released when the row itself is deleted.
func TestStrictIntentSurvivesDeactivation(t *testing.T) {
	pm := NewProxyManager([]model.ProxyPool{})
	pm.UpdatePools([]model.ProxyPool{pool("socks5://127.0.0.1:1", false, true)})
	if _, err := pm.GetHTTPClient(false).Get("http://example.invalid"); err == nil ||
		!strings.Contains(err.Error(), "strict proxy mode") {
		t.Fatalf("deactivated strict pool must still refuse direct, got %v", err)
	}
	pm.UpdatePools(nil)
	if _, err := pm.GetHTTPClient(false).Get("http://127.0.0.1:1/none"); err != nil &&
		strings.Contains(err.Error(), "strict proxy mode") {
		t.Fatalf("deleting the pool must release strict mode, got %v", err)
	}
}
