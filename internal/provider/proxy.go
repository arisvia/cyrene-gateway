package provider

import (
	"net/http"
	"net/url"
	"sync/atomic"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

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
// If no proxies are configured, returns a standard client.
func (pm *ProxyManager) GetHTTPClient() *http.Client {
	proxyURL := pm.NextProxy()
	if proxyURL == "" {
		return &http.Client{}
	}

	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return &http.Client{}
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(parsed),
	}
	return &http.Client{Transport: transport}
}
