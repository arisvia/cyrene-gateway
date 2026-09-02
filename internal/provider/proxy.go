package provider

import (
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// defaultProxyTimeout bounds every request issued through GetHTTPClient so the
// client is never left with an unbounded timeout. Callers override it with the
// request-scoped timeout immediately after (handler/chat.go getHTTPClient).
const defaultProxyTimeout = 2 * time.Minute

// ProxyManager handles outbound proxy rotation for upstream requests.
type ProxyManager struct {
	pools   []model.ProxyPool
	counter atomic.Int64
}

// NewProxyManager creates a proxy manager from active proxy pools.
func NewProxyManager(pools []model.ProxyPool) *ProxyManager {
	var active []model.ProxyPool
	for _, p := range pools {
		if p.IsActive && p.Data.ProxyURL != "" {
			active = append(active, p)
		}
	}
	return &ProxyManager{pools: active}
}

// UpdatePools refreshes the active proxy pool list.
func (pm *ProxyManager) UpdatePools(pools []model.ProxyPool) {
	var active []model.ProxyPool
	for _, p := range pools {
		if p.IsActive && p.Data.ProxyURL != "" {
			active = append(active, p)
		}
	}
	pm.pools = active
}

// HasProxies returns true if any active proxy pools are configured.
func (pm *ProxyManager) HasProxies() bool {
	return len(pm.pools) > 0
}

// NextProxy returns the next proxy URL using round-robin rotation.
// Returns empty string if no proxies are available.
func (pm *ProxyManager) NextProxy() string {
	if len(pm.pools) == 0 {
		return ""
	}
	idx := pm.counter.Add(1) - 1
	pool := pm.pools[idx%int64(len(pm.pools))]
	return pool.Data.ProxyURL
}

// GetHTTPClient returns an http.Client configured with the next available proxy.
// The returned client reuses the SSRF-safe transport (dial-time IP blocking plus
// redirect validation) so that enabling proxy rotation cannot bypass SSRF policy.
// The proxy endpoint itself is exempt from dial-time blocking: with Transport.Proxy
// set, DialContext only ever sees the proxy's address — never the request target —
// so target SSRF is enforced by CheckRedirect plus the proxy's own egress rules.
// This keeps standard local proxy pools (e.g. Clash/V2Ray on 127.0.0.1) working
// with allowPrivate=false.
// If no proxies are configured (or the URL is invalid), returns a standard
// SSRF-safe client with a bounded default timeout.
func (pm *ProxyManager) GetHTTPClient(allowPrivate bool) *http.Client {
	client := SafeHTTPClient(defaultProxyTimeout, allowPrivate)

	proxyURL := pm.NextProxy()
	if proxyURL == "" {
		return client
	}

	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return client
	}

	exempt := proxyExemptHosts(parsed)
	transport := &http.Transport{
		DialContext:           safeDialer(allowPrivate, exempt).DialContext,
		Proxy:                 http.ProxyURL(parsed),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	client.Transport = transport
	return client
}

// proxyExemptHosts returns the dial-time permit-list for a proxy endpoint: the
// hostname as spelled plus every IP it resolves to, because net.Dialer.Control
// receives resolved IP literals (127.0.0.1, ::1, …), never the hostname — a
// "localhost:7890" or "proxy.corp.lan" spelling must dial the same as a literal.
// Resolution failure leaves the hostname-only set: the dial then fails closed.
func proxyExemptHosts(parsed *url.URL) map[string]bool {
	host := parsed.Hostname()
	exempt := map[string]bool{host: true}
	if ips, err := net.LookupIP(host); err == nil {
		for _, ip := range ips {
			exempt[ip.String()] = true
		}
	}
	return exempt
}
