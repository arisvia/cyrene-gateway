package provider

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"sync"
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
	mu         sync.RWMutex
	pools      []model.ProxyPool
	strict     bool
	counter    atomic.Int64
	transports map[proxyRoute]*safeTransport
}

// An empty proxyURL selects the default route, including environment proxies.
// Private-access policy must never be shared across transport cache entries.
type proxyRoute struct {
	proxyURL     string
	allowPrivate bool
}

// NewProxyManager creates a proxy manager from active proxy pools.
func NewProxyManager(pools []model.ProxyPool) *ProxyManager {
	pm := &ProxyManager{}
	pm.UpdatePools(pools)
	return pm
}

// UpdatePools refreshes active pools and closes idle connections on routes no
// longer selectable. In-flight requests are not interrupted. A pool flagged
// StrictProxy makes the manager refuse silent direct fallback even after that
// pool is deactivated: the operator's "never go bare" intent survives until the
// pool row itself is deleted.
func (pm *ProxyManager) UpdatePools(pools []model.ProxyPool) {
	var active []model.ProxyPool
	strict := false
	routes := make(map[string]bool)
	for _, p := range pools {
		if p.Data.StrictProxy {
			strict = true
		}
		if p.IsActive && p.Data.ProxyURL != "" {
			active = append(active, p)
			route, _ := parseProxyRoute(p.Data.ProxyURL)
			routes[route] = true
		}
	}
	if len(active) == 0 {
		routes[""] = true
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.pools = active
	pm.strict = strict
	for route, transport := range pm.transports {
		if !routes[route.proxyURL] {
			delete(pm.transports, route)
			transport.CloseIdleConnections()
		}
	}
}

// HasProxies returns true if any active proxy pools are configured.
func (pm *ProxyManager) HasProxies() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.pools) > 0
}

// strictFailTransport is handed out when a strict proxy pool exists but no
// proxy route is selectable: every request fails at the transport layer with
// an explicit refusal instead of silently dialing upstream direct.
type strictFailTransport struct{}

func (strictFailTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("strict proxy mode: no active proxy pool, refusing direct connection")
}

func (strictFailTransport) CloseIdleConnections() {}

// NextProxy returns the next proxy URL using round-robin rotation.
// Returns empty string if no proxies are available.
func (pm *ProxyManager) NextProxy() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.nextProxyLocked()
}

func (pm *ProxyManager) nextProxyLocked() string {
	if len(pm.pools) == 0 {
		return ""
	}
	idx := pm.counter.Add(1) - 1
	pool := pm.pools[idx%int64(len(pm.pools))]
	return pool.Data.ProxyURL
}

// GetHTTPClient returns an independent client with a bounded, caller-adjustable
// timeout. Transports are reused within this manager by route and SSRF policy,
// including the default route when no valid proxy is configured. Target checks
// remain wrapped around the transport, not just attached to redirect callbacks.
func (pm *ProxyManager) GetHTTPClient(allowPrivate bool) *http.Client {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	proxyURL, parsed := parseProxyRoute(pm.nextProxyLocked())
	if proxyURL == "" && pm.strict {
		return &http.Client{Transport: strictFailTransport{}, Timeout: defaultProxyTimeout}
	}
	route := proxyRoute{proxyURL: proxyURL, allowPrivate: allowPrivate}
	transport := pm.transports[route]
	if transport == nil {
		transport = newSafeTransport(allowPrivate, parsed)
		if pm.transports == nil {
			pm.transports = make(map[proxyRoute]*safeTransport)
		}
		pm.transports[route] = transport
	}
	return &http.Client{Transport: transport, Timeout: defaultProxyTimeout}
}

func parseProxyRoute(rawURL string) (string, *url.URL) {
	if rawURL == "" {
		return "", nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", nil
	}
	return parsed.String(), parsed
}

// proxyEndpoint is used only for the operator-selected proxy, never a target.
// Its DNS is resolved normally on each new connection, with no cached IP list.
func proxyEndpoint(u *url.URL) (string, error) {
	port := u.Port()
	defaultPort := ""
	switch u.Scheme {
	case "http", "":
		defaultPort = "80"
	case "https":
		defaultPort = "443"
	case "socks5", "socks5h":
		defaultPort = "1080"
	default:
		return "", errors.New("unsupported proxy scheme; expected http, https or socks5")
	}
	if u.Hostname() == "" || u.Opaque != "" {
		return "", errors.New("invalid proxy endpoint")
	}
	if port == "" {
		port = defaultPort
	}
	if n, err := strconv.ParseUint(port, 10, 16); err != nil || n == 0 {
		return "", errors.New("invalid proxy port")
	}
	return net.JoinHostPort(u.Hostname(), port), nil
}

// ValidateProxyURL parses and validates the scheme, host, and port for an operator proxy endpoint.
func ValidateProxyURL(rawURL string) (*url.URL, error) {
	if rawURL == "" {
		return nil, errors.New("proxy URL cannot be empty")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}
	if _, err := proxyEndpoint(parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (t *safeTransport) dialProxyTunnel(ctx context.Context, network string, proxy *url.URL, target string) (result net.Conn, err error) {
	endpoint, err := proxyEndpoint(proxy)
	if err != nil {
		return nil, err
	}
	// Only this exact configured endpoint bypasses the private-IP policy. A
	// target is never passed to this dialer, even if NO_PROXY selects direct.
	dialer := &net.Dialer{Timeout: safeConnectTimeout, KeepAlive: 30 * time.Second}
	conn, err := dialer.DialContext(ctx, network, endpoint)
	if err != nil {
		return nil, fmt.Errorf("dial proxy: %w", err)
	}
	closed := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		conn.Close()
		close(closed)
	})
	defer func() {
		if !stop() {
			<-closed
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		if err != nil {
			conn.Close()
			result = nil
		} else if err = conn.SetDeadline(time.Time{}); err != nil {
			conn.Close()
			result = nil
		}
	}()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return nil, err
		}
	}
	var tunnel net.Conn = conn
	if proxy.Scheme == "https" {
		config := &tls.Config{}
		if t.transport.TLSClientConfig != nil {
			config = t.transport.TLSClientConfig.Clone()
		}
		config.ServerName = proxy.Hostname()
		config.NextProtos = []string{"http/1.1"}
		secure := tls.Client(conn, config)
		if err := secure.HandshakeContext(ctx); err != nil {
			return nil, fmt.Errorf("proxy TLS handshake: %w", err)
		}
		tunnel = secure
	}
	if proxy.Scheme == "socks5" || proxy.Scheme == "socks5h" {
		if err := socks5Connect(tunnel, proxy.User, target); err != nil {
			return nil, fmt.Errorf("proxy SOCKS5 tunnel: %w", err)
		}
		return tunnel, nil
	}
	return httpConnect(tunnel, proxy.User, target)
}

// HTTP upstreams also require CONNECT: an absolute URI would delegate target
// DNS back to the proxy. CONNECT:80 rejection is an explicit error, not a reason
// to fall back to unsafe forwarding. Host and TLS SNI stay with net/http.
func httpConnect(conn net.Conn, user *url.Userinfo, target string) (net.Conn, error) {
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: target}, Host: target, Header: make(http.Header)}
	if user != nil {
		password, _ := user.Password()
		req.SetBasicAuth(user.Username(), password)
		req.Header.Set("Proxy-Authorization", req.Header.Get("Authorization"))
		req.Header.Del("Authorization")
	}
	if err := req.Write(conn); err != nil {
		return nil, fmt.Errorf("write proxy CONNECT: %w", err)
	}
	// Bound untrusted response headers. Preserve any bytes read ahead (including
	// tunnel data) and resume the original connection after the limited reader.
	reader := bufio.NewReader(io.LimitReader(conn, 64<<10))
	resp, err := http.ReadResponse(reader, req)
	if err != nil {
		return nil, fmt.Errorf("read proxy CONNECT: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("proxy CONNECT %s failed: %s (safe tunnel required; no forwarding fallback)", target, resp.Status)
	}
	return &tunnelReaderConn{Conn: conn, reader: io.MultiReader(reader, conn)}, nil
}

type tunnelReaderConn struct {
	net.Conn
	reader io.Reader
}

func (c *tunnelReaderConn) Read(p []byte) (int, error) { return c.reader.Read(p) }

// socks5Connect implements RFC 1928 CONNECT and optional RFC 1929 auth using
// only IP address types. Even socks5h must not delegate target DNS to the proxy.
func socks5Connect(conn net.Conn, user *url.Userinfo, target string) error {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || ip.Zone() != "" {
		return errors.New("SOCKS5 target must be an unscoped IP")
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n == 0 {
		return errors.New("invalid SOCKS5 target port")
	}
	greeting := []byte{5, 1, 0}
	var username, password string
	if user != nil {
		username = user.Username()
		password, _ = user.Password()
		if len(username) == 0 || len(username) > 255 || len(password) > 255 {
			return errors.New("invalid SOCKS5 credential length")
		}
		greeting = []byte{5, 2, 0, 2}
	}
	if _, err := conn.Write(greeting); err != nil {
		return err
	}
	var auth [2]byte
	if _, err := io.ReadFull(conn, auth[:]); err != nil {
		return err
	}
	if auth[0] != 5 {
		return errors.New("invalid SOCKS5 version")
	}
	switch auth[1] {
	case 0:
	case 2:
		if user == nil {
			return errors.New("SOCKS5 requested unconfigured authentication")
		}
		message := append([]byte{1, byte(len(username))}, username...)
		message = append(message, byte(len(password)))
		message = append(message, password...)
		if _, err := conn.Write(message); err != nil {
			return err
		}
		if _, err := io.ReadFull(conn, auth[:]); err != nil {
			return err
		}
		if auth != [2]byte{1, 0} {
			return errors.New("SOCKS5 authentication rejected")
		}
	default:
		return errors.New("SOCKS5 authentication method rejected")
	}
	ip = ip.Unmap()
	addressType := byte(4)
	if ip.Is4() {
		addressType = 1
	}
	request := append([]byte{5, 1, 0, addressType}, ip.AsSlice()...)
	request = append(request, byte(n>>8), byte(n))
	if _, err := conn.Write(request); err != nil {
		return err
	}
	var reply [4]byte
	if _, err := io.ReadFull(conn, reply[:]); err != nil {
		return err
	}
	if reply[0] != 5 || reply[1] != 0 || reply[2] != 0 {
		return fmt.Errorf("SOCKS5 CONNECT rejected: %v", reply)
	}
	size := 0
	switch reply[3] {
	case 1:
		size = 4
	case 4:
		size = 16
	case 3:
		var length [1]byte
		if _, err := io.ReadFull(conn, length[:]); err != nil {
			return err
		}
		size = int(length[0])
	default:
		return errors.New("invalid SOCKS5 reply address type")
	}
	_, err = io.CopyN(io.Discard, conn, int64(size+2))
	return err
}
