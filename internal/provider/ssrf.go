package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrPrivateNetworkBlocked = errors.New("access to private, loopback, or link-local address is blocked by SSRF policy")
	ErrUnsupportedScheme     = errors.New("url scheme must be http or https")
)

const safeConnectTimeout = 10 * time.Second

// ValidateUpstreamURL validates a user-provided BaseURL or PanelURL against SSRF risks.
// Configuration may name an offline private service; actual requests always resolve
// and pin their destination, even when private access is enabled.
func ValidateUpstreamURL(rawURL string, allowPrivate bool) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), safeConnectTimeout)
	defer cancel()
	if err := validateTargetURL(ctx, u, allowPrivate); err != nil {
		return nil, err
	}
	return u, nil
}

func validateTargetURL(ctx context.Context, u *url.URL, allowPrivate bool) error {
	if allowPrivate {
		_, err := targetHostname(u, true)
		return err
	}
	_, err := resolveTarget(ctx, u, false)
	return err
}

func targetHostname(u *url.URL, allowPrivate bool) (string, error) {
	if u == nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "", ErrUnsupportedScheme
	}
	if u.Opaque != "" {
		return "", errors.New("opaque target URL is not allowed")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" {
		return "", errors.New("empty host")
	}
	if !allowPrivate && (host == "localhost" ||
		strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") ||
		strings.HasSuffix(host, ".lan") ||
		strings.HasSuffix(host, ".home.arpa")) {
		return "", fmt.Errorf("%w: reserved local domain %s", ErrPrivateNetworkBlocked, host)
	}
	return host, nil
}

// resolveTarget is the sole target lookup for a request. Every returned IP must
// satisfy policy; neither a direct dial nor a proxy may resolve the name again.
func resolveTarget(ctx context.Context, u *url.URL, allowPrivate bool) ([]net.IPAddr, error) {
	host, err := targetHostname(u, allowPrivate)
	if err != nil {
		return nil, err
	}
	var ips []net.IPAddr
	if addr, err := netip.ParseAddr(host); err == nil {
		ips = []net.IPAddr{{IP: net.IP(addr.AsSlice()), Zone: addr.Zone()}}
	} else {
		ips, err = net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve target %s: %w", host, err)
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("resolve target %s: no addresses", host)
	}
	for _, ip := range ips {
		if !allowPrivate && isBlockedIP(ip.IP) {
			return nil, fmt.Errorf("%w: resolved to %s", ErrPrivateNetworkBlocked, ip.String())
		}
	}
	return ips, nil
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	// Explicitly check IPv4 cloud metadata range 169.254.0.0/16 and CGNAT 100.64.0.0/10.
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		if ip4[0] == 100 && (ip4[1]&0xC0) == 64 {
			return true
		}
	}
	return false
}

// SafeDialerControl blocks private destinations at the socket boundary. There
// are no shared proxy-IP exemptions, including on a NO_PROXY direct route.
func SafeDialerControl(allowPrivate bool) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		if allowPrivate {
			return nil
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}
		ip, err := netip.ParseAddr(host)
		if err != nil || isBlockedIP(net.IP(ip.AsSlice())) {
			return ErrPrivateNetworkBlocked
		}
		return nil
	}
}

func safeDialer(allowPrivate bool) *net.Dialer {
	return &net.Dialer{
		Timeout: safeConnectTimeout, KeepAlive: 30 * time.Second,
		Control: SafeDialerControl(allowPrivate),
	}
}

type safeDialPlanKey struct{}

type safeDialPlan struct {
	requestContext context.Context
	addresses      []string
	proxy          *url.URL
}

// Each wrapper owns its transports. Environment proxy selection happens per
// request, with distinct pools for actual proxy routes and direct connections.
// The underlying Transport sees the original URL (Host/SNI), but Proxy is nil:
// its DialContext establishes an IP-bound tunnel rather than a forward request.
type safeTransport struct {
	transport    *http.Transport
	allowPrivate bool
	proxy        func(*http.Request) (*url.URL, error)
	mu           sync.Mutex
	routes       map[string]*http.Transport
}

func (t *safeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	transport, plan, err := t.prepare(req)
	if err != nil {
		if req.Body != nil {
			req.Body.Close()
		}
		return nil, err
	}
	ctx := context.WithValue(req.Context(), safeDialPlanKey{}, plan)
	wireReq := req.WithContext(ctx)
	if plan.proxy != nil && req.Header.Get("Proxy-Authorization") != "" {
		wireReq = wireReq.Clone(ctx)
		wireReq.Header.Del("Proxy-Authorization")
	}
	return transport.RoundTrip(wireReq)
}

func (t *safeTransport) prepare(req *http.Request) (*http.Transport, *safeDialPlan, error) {
	ctx, cancel := context.WithTimeout(req.Context(), safeConnectTimeout)
	defer cancel()
	ips, err := resolveTarget(ctx, req.URL, t.allowPrivate)
	if err != nil {
		return nil, nil, err
	}
	if req.Host != "" && req.Host != req.URL.Host {
		target := *req.URL
		target.Host = req.Host
		if err := validateTargetURL(ctx, &target, t.allowPrivate); err != nil {
			return nil, nil, err
		}
	}
	proxyURL, err := t.proxy(req)
	if err != nil {
		return nil, nil, err
	}
	if proxyURL != nil {
		if _, err := proxyEndpoint(proxyURL); err != nil {
			return nil, nil, err
		}
	}
	port := req.URL.Port()
	if port == "" {
		port = "80"
		if req.URL.Scheme == "https" {
			port = "443"
		}
	}
	plan := &safeDialPlan{requestContext: req.Context(), proxy: proxyURL}
	for _, ip := range ips {
		plan.addresses = append(plan.addresses, net.JoinHostPort(ip.String(), port))
	}
	if proxyURL == nil {
		return t.transport, plan, nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	key := proxyURL.String()
	transport := t.routes[key]
	if transport == nil {
		transport = t.transport.Clone()
		t.routes[key] = transport
	}
	return transport, plan, nil
}

func (t *safeTransport) dialContext(ctx context.Context, network, _ string) (net.Conn, error) {
	plan, ok := ctx.Value(safeDialPlanKey{}).(*safeDialPlan)
	if !ok {
		return nil, errors.New("missing validated target dial plan")
	}
	// net/http detaches request cancellation and deadlines when dialing for reuse.
	deadline := time.Now().Add(safeConnectTimeout)
	if requestDeadline, ok := plan.requestContext.Deadline(); ok && requestDeadline.Before(deadline) {
		deadline = requestDeadline
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	stop := context.AfterFunc(plan.requestContext, cancel)
	defer stop()
	deadline, _ = ctx.Deadline()
	var err error
	for i, address := range plan.addresses {
		if plan.requestContext.Err() != nil {
			return nil, plan.requestContext.Err()
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		attemptCtx, cancelAttempt := context.WithTimeout(ctx, time.Until(deadline)/time.Duration(len(plan.addresses)-i))
		var conn net.Conn
		if plan.proxy == nil {
			conn, err = safeDialer(t.allowPrivate).DialContext(attemptCtx, network, address)
		} else {
			conn, err = t.dialProxyTunnel(attemptCtx, network, plan.proxy, address)
		}
		cancelAttempt()
		if err == nil {
			return conn, nil
		}
	}
	return nil, err
}

func (t *safeTransport) CloseIdleConnections() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.transport.CloseIdleConnections()
	for _, transport := range t.routes {
		transport.CloseIdleConnections()
	}
}

func newSafeTransport(allowPrivate bool, proxyURL *url.URL) *safeTransport {
	proxy := http.ProxyFromEnvironment
	if proxyURL != nil {
		copyURL := *proxyURL
		proxy = http.ProxyURL(&copyURL)
	}
	t := &safeTransport{
		allowPrivate: allowPrivate, proxy: proxy, routes: make(map[string]*http.Transport),
		transport: &http.Transport{
			ForceAttemptHTTP2: true, MaxIdleConns: 100,
			IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: safeConnectTimeout,
			ExpectContinueTimeout: time.Second,
		},
	}
	t.transport.DialContext = t.dialContext
	return t
}

// SafeHTTPClient returns a fresh client and transport with IP-bound target
// validation. HTTP_PROXY / HTTPS_PROXY and NO_PROXY follow net/http semantics;
// local proxy endpoints are permitted, never private targets unless opted in.
// Redirects pass through the same safety wrapper and standard redirect limit.
func SafeHTTPClient(timeout time.Duration, allowPrivate bool) *http.Client {
	return &http.Client{Transport: newSafeTransport(allowPrivate, nil), Timeout: timeout}
}
