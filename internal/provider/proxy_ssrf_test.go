package provider

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestSafeDialerControlHasNoProxyExemptions(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8080", "169.254.169.254:80", "[fe80::1%eth0]:80"} {
		if err := SafeDialerControl(false)("tcp", addr, nil); !errors.Is(err, ErrPrivateNetworkBlocked) {
			t.Errorf("private address %s must stay blocked: %v", addr, err)
		}
		if err := SafeDialerControl(true)("tcp", addr, nil); err != nil {
			t.Errorf("allowPrivate must bypass blocking: %v", err)
		}
	}
}

func testProxyPools(rawURL string) []model.ProxyPool {
	return []model.ProxyPool{{
		ID: "local", IsActive: true,
		Data: model.ProxyPoolData{Name: "local", ProxyURL: rawURL},
	}}
}

// DNS is deliberately unavailable: policy must fail closed without consulting
// the machine's DNS configuration or making any external network requests.
func denyTestDNS(t *testing.T) {
	t.Helper()
	previous := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("test DNS unavailable")
		},
	}
	t.Cleanup(func() { net.DefaultResolver = previous })
}

func TestProxyClientValidatesEveryTarget(t *testing.T) {
	runProxyTargetPolicy(t, false)
}

func TestSafeHTTPClientHonorsEnvironmentProxy(t *testing.T) {
	// net/http caches environment proxy settings process-wide. Isolate this
	// test rather than depending on test order or replacing the safety wrapper.
	if os.Getenv("CYRENE_TEST_ENV_PROXY") == "1" {
		runProxyTargetPolicy(t, true)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSafeHTTPClientHonorsEnvironmentProxy$", "-test.v")
	cmd.Env = append(os.Environ(), "CYRENE_TEST_ENV_PROXY=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("environment proxy subprocess: %v\n%s", err, out)
	}
}

func runProxyTargetPolicy(t *testing.T, environment bool) {
	t.Helper()
	stubTestDNS(t, map[string][]net.IP{
		"public.example.":  {net.ParseIP("93.184.216.34")},
		"private.example.": {net.ParseIP("10.0.0.1")},
		"mixed.example.":   {net.ParseIP("93.184.216.34"), net.ParseIP("fd00::1")},
	})
	var requests, connects atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			connects.Add(1)
			conn, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer conn.Close()
			if _, err := rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
				return
			}
			if err := rw.Flush(); err != nil {
				return
			}
			for {
				inner, err := http.ReadRequest(rw.Reader)
				if err != nil {
					return
				}
				requests.Add(1)
				var resp *http.Response
				if inner.URL.Path == "/redirect" {
					resp = &http.Response{
						StatusCode: http.StatusFound,
						Header:     http.Header{"Location": {"http://169.254.169.254/latest/meta-data/"}},
						Proto:      "HTTP/1.1",
						ProtoMajor: 1,
						ProtoMinor: 1,
					}
				} else {
					resp = &http.Response{
						StatusCode: http.StatusNoContent,
						Proto:      "HTTP/1.1",
						ProtoMajor: 1,
						ProtoMinor: 1,
						Header:     make(http.Header),
					}
				}
				writeErr := resp.Write(conn)
				inner.Body.Close()
				if writeErr != nil {
					return
				}
			}
		}
		requests.Add(1)
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()

	pm := NewProxyManager(testProxyPools(proxy.URL))
	newClient := pm.GetHTTPClient
	if environment {
		for _, key := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"} {
			t.Setenv(key, proxy.URL)
		}
		for _, key := range []string{"NO_PROXY", "no_proxy", "ALL_PROXY", "all_proxy", "REQUEST_METHOD"} {
			t.Setenv(key, "")
		}
		newClient = func(allowPrivate bool) *http.Client {
			return SafeHTTPClient(5*time.Second, allowPrivate)
		}
	}

	for _, tt := range []struct {
		name         string
		target       string
		allowPrivate bool
		wantErr      bool
		sentinel     error
		requests     int64
		connects     int64
	}{
		{"proxy-address-is-not-a-target-exemption", proxy.URL, false, true, ErrPrivateNetworkBlocked, 0, 0},
		{"private-first-request", "http://10.0.0.1/", false, true, ErrPrivateNetworkBlocked, 0, 0},
		{"metadata-first-request", "http://169.254.169.254/latest/meta-data/", false, true, ErrPrivateNetworkBlocked, 0, 0},
		{"ipv6-loopback", "http://[::1]/", false, true, ErrPrivateNetworkBlocked, 0, 0},
		{"scoped-ipv6", "http://[fe80::1%25eth0]/", false, true, ErrPrivateNetworkBlocked, 0, 0},
		{"local-domain", "http://service.internal/", false, true, ErrPrivateNetworkBlocked, 0, 0},
		{"absolute-localhost", "http://localhost./", false, true, ErrPrivateNetworkBlocked, 0, 0},
		{"scheme", "ftp://93.184.216.34/", false, true, ErrUnsupportedScheme, 0, 0},
		{"empty-host", "http:///path", false, true, nil, 0, 0},
		{"dns-failure", "http://unresolved.invalid/", false, true, nil, 0, 0},
		{"dns-public", "http://public.example/", false, false, nil, 1, 1},
		{"dns-private", "http://private.example/", false, true, ErrPrivateNetworkBlocked, 0, 0},
		{"dns-mixed", "http://mixed.example/", false, true, ErrPrivateNetworkBlocked, 0, 0},
		{"dns-private-connect", "https://private.example/", false, true, ErrPrivateNetworkBlocked, 0, 0},
		{"public-http", "http://93.184.216.34/", false, false, nil, 1, 1},
		{"metadata-redirect", "http://93.184.216.34/redirect", false, true, ErrPrivateNetworkBlocked, 1, 1},
		{"private-connect", "https://10.0.0.1/", false, true, ErrPrivateNetworkBlocked, 0, 0},
		{"metadata-connect", "https://169.254.169.254/", false, true, ErrPrivateNetworkBlocked, 0, 0},
		{"public-connect", "https://93.184.216.34/", false, true, nil, 0, 1},
		{"development-http", "http://10.0.0.1/", true, false, nil, 1, 1},
		{"development-dns", "http://unresolved.invalid/", true, true, nil, 0, 0},
		{"development-redirect", "http://93.184.216.34/redirect", true, false, nil, 2, 2},
		{"development-connect", "https://10.0.0.1/", true, true, nil, 0, 1},
		{"development-scheme", "ftp://93.184.216.34/", true, true, ErrUnsupportedScheme, 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newClient(tt.allowPrivate)
			client.Timeout = 5 * time.Second
			defer client.CloseIdleConnections()
			before, beforeConnect := requests.Load(), connects.Load()
			resp, err := client.Get(tt.target)
			if resp != nil {
				resp.Body.Close()
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("request error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.sentinel != nil && !errors.Is(err, tt.sentinel) {
				t.Errorf("expected %v, got %v", tt.sentinel, err)
			}
			if got := requests.Load() - before; got != tt.requests {
				t.Errorf("proxy received %d requests, want %d", got, tt.requests)
			}
			if got := connects.Load() - beforeConnect; got != tt.connects {
				t.Errorf("proxy received %d CONNECTs, want %d", got, tt.connects)
			}
		})
	}
}

func TestProxyClientRejectsAlternateWireTargets(t *testing.T) {
	var hits atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()
	client := NewProxyManager(testProxyPools(proxy.URL)).GetHTTPClient(false)
	defer client.CloseIdleConnections()
	for _, opaque := range []bool{false, true} {
		req, err := http.NewRequest(http.MethodGet, "http://93.184.216.34/", nil)
		if err != nil {
			t.Fatal(err)
		}
		if opaque {
			req.URL.Opaque = "//169.254.169.254/latest/meta-data/"
		} else {
			req.Host = "169.254.169.254"
		}
		resp, err := client.Do(req)
		if resp != nil {
			resp.Body.Close()
		}
		if err == nil || hits.Load() != 0 {
			t.Errorf("alternate wire target must be blocked before proxy: opaque=%v, hits=%d, err=%v", opaque, hits.Load(), err)
		}
	}
}

func TestGetHTTPClientWithoutProxiesIsSafe(t *testing.T) {
	client := NewProxyManager(nil).GetHTTPClient(false)
	defer client.CloseIdleConnections()
	if client.Timeout <= 0 {
		t.Fatalf("expected bounded timeout, got %v", client.Timeout)
	}
	resp, err := client.Get("http://127.0.0.1:1/")
	if resp != nil {
		resp.Body.Close()
	}
	if !errors.Is(err, ErrPrivateNetworkBlocked) {
		t.Fatalf("expected SSRF protection, got %v", err)
	}
}

func TestProxyManagerReusesTransportNotClient(t *testing.T) {
	for _, route := range []string{"", "http://127.0.0.1:7890"} {
		t.Run(route, func(t *testing.T) {
			pm := NewProxyManager(testProxyPools(route))
			for _, allow := range []bool{false, true} {
				first, second := pm.GetHTTPClient(allow), pm.GetHTTPClient(allow)
				t.Cleanup(first.CloseIdleConnections)
				t.Cleanup(second.CloseIdleConnections)
				if first == second {
					t.Fatal("callers must receive independent clients")
				}
				first.Timeout = time.Second
				second.Timeout = 3 * time.Second
				if first.Timeout != time.Second || pm.GetHTTPClient(allow).Timeout != defaultProxyTimeout {
					t.Error("request timeouts leaked between clients")
				}
				if first.Transport != second.Transport {
					t.Error("same route and policy must share transport")
				}
				if first.Transport == pm.GetHTTPClient(!allow).Transport {
					t.Error("different SSRF policies must not share transport")
				}
				other := NewProxyManager(testProxyPools(route)).GetHTTPClient(allow)
				t.Cleanup(other.CloseIdleConnections)
				if first.Transport == other.Transport {
					t.Error("transport cache must be scoped to the manager")
				}
			}
		})
	}
}

func TestSafeHTTPClientDoesNotShareTransport(t *testing.T) {
	first, second := SafeHTTPClient(time.Second, false), SafeHTTPClient(time.Second, false)
	defer first.CloseIdleConnections()
	defer second.CloseIdleConnections()
	if first == second || first.Transport == second.Transport {
		t.Fatal("standalone clients must not share mutable client or transport state")
	}
}

// closeTrackingListener wraps a net.Listener to signal the closed channel
// whenever an accepted connection is closed. This is needed because Go's
// ConnState callback never fires StateClosed after a connection is Hijacked.
type closeTrackingListener struct {
	net.Listener
	closed chan struct{}
}

func (l *closeTrackingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &closeNotifyConn{Conn: c, closed: l.closed}, nil
}

// closeNotifyConn wraps a net.Conn to signal the closed channel on Close.
type closeNotifyConn struct {
	net.Conn
	closed  chan struct{}
	closeFn sync.Once
}

func (c *closeNotifyConn) Close() error {
	err := c.Conn.Close()
	c.closeFn.Do(func() { c.closed <- struct{}{} })
	return err
}

func TestProxyManagerUpdatePoolsClosesRetiredIdleConnections(t *testing.T) {
	for _, direct := range []bool{false, true} {
		t.Run(map[bool]string{false: "proxy", true: "direct"}[direct], func(t *testing.T) {
			closed := make(chan struct{}, 8)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodConnect {
					conn, rw, err := w.(http.Hijacker).Hijack()
					if err != nil {
						return
					}
					defer conn.Close()
					if _, err := rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
						return
					}
					if err := rw.Flush(); err != nil {
						return
					}
					for {
						inner, err := http.ReadRequest(rw.Reader)
						if err != nil {
							return
						}
						resp := &http.Response{
							StatusCode: http.StatusNoContent,
							Proto:      "HTTP/1.1",
							ProtoMajor: 1,
							ProtoMinor: 1,
							Header:     make(http.Header),
						}
						writeErr := resp.Write(conn)
						inner.Body.Close()
						if writeErr != nil {
							return
						}
					}
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			if direct {
				// Non-hijacked connections: ConnState fires StateClosed reliably.
				server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
					if state == http.StateClosed {
						closed <- struct{}{}
					}
				}
				server.Start()
			} else {
				// Hijacked connections: Go's ConnState never fires StateClosed
				// after Hijack. Wrap the listener to track raw TCP close instead.
				inner := server.Listener
				server.Listener = &closeTrackingListener{Listener: inner, closed: closed}
				server.Start()
			}
			defer server.Close()
			pools, target := testProxyPools(server.URL), "http://93.184.216.34/"
			if direct {
				pools, target = nil, server.URL
			}
			pm := NewProxyManager(pools)
			client := pm.GetHTTPClient(direct)
			defer client.CloseIdleConnections()
			resp, err := client.Get(target)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			pm.UpdatePools(testProxyPools("http://127.0.0.1:1"))
			select {
			case <-closed:
			case <-time.After(3 * time.Second):
				t.Error("removing route did not close its idle connection")
			}
			pm.UpdatePools(pools)
			if client.Transport == pm.GetHTTPClient(direct).Transport {
				t.Error("retired route must not remain in cache")
			}
		})
	}
}

func TestProxyManagerRetainsUnchangedRouteConnections(t *testing.T) {
	peers := make(chan string, 4)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			conn, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer conn.Close()
			if _, err := rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
				return
			}
			if err := rw.Flush(); err != nil {
				return
			}
			for {
				inner, err := http.ReadRequest(rw.Reader)
				if err != nil {
					return
				}
				peers <- r.RemoteAddr
				resp := &http.Response{
					StatusCode: http.StatusNoContent,
					Proto:      "HTTP/1.1",
					ProtoMajor: 1,
					ProtoMinor: 1,
					Header:     make(http.Header),
				}
				writeErr := resp.Write(conn)
				inner.Body.Close()
				if writeErr != nil {
					return
				}
			}
		}
		peers <- r.RemoteAddr
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()
	pm := NewProxyManager(testProxyPools(proxy.URL))
	var firstPeer string
	for range 3 {
		client := pm.GetHTTPClient(false)
		defer client.CloseIdleConnections()
		resp, err := client.Get("http://93.184.216.34/")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		peer := <-peers
		if firstPeer != "" && peer != firstPeer {
			t.Errorf("same route opened a new connection: %s -> %s", firstPeer, peer)
		}
		firstPeer = peer
		pm.UpdatePools(testProxyPools(proxy.URL))
	}
}

func TestProxyManager_ConcurrentUpdateAndNext(t *testing.T) {
	pm := NewProxyManager(testProxyPools("http://127.0.0.1:7890"))
	done := make(chan struct{})
	go func() {
		for range 100 {
			pm.UpdatePools(nil)
			pm.UpdatePools(testProxyPools("http://127.0.0.1:7891"))
		}
		done <- struct{}{}
	}()
	for range 4 {
		go func() {
			for i := range 100 {
				_ = pm.HasProxies()
				_ = pm.NextProxy()
				client := pm.GetHTTPClient(i%2 == 0)
				client.Timeout = time.Duration(i+1) * time.Second
			}
			done <- struct{}{}
		}()
	}
	for range 5 {
		<-done
	}
	pm.UpdatePools(nil)
}

func TestProxyManagerRoundRobinTransportReuse(t *testing.T) {
	pools := append(testProxyPools("http://127.0.0.1:7890"), testProxyPools("http://127.0.0.1:7891")...)
	pm := NewProxyManager(pools)
	first, second, third := pm.GetHTTPClient(false), pm.GetHTTPClient(false), pm.GetHTTPClient(false)
	defer first.CloseIdleConnections()
	defer second.CloseIdleConnections()
	defer third.CloseIdleConnections()
	if first.Transport == second.Transport || first.Transport != third.Transport {
		t.Fatal("round-robin routes need distinct, reusable transports")
	}
	pm.UpdatePools(pools[1:])
	if pm.GetHTTPClient(false).Transport != second.Transport {
		t.Error("updating pools discarded a still-active transport")
	}
}

func TestProxyClientKeepsRedirectLimit(t *testing.T) {
	var hits atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			conn, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer conn.Close()
			if _, err := rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
				return
			}
			if err := rw.Flush(); err != nil {
				return
			}
			for {
				inner, err := http.ReadRequest(rw.Reader)
				if err != nil {
					return
				}
				hits.Add(1)
				resp := &http.Response{
					StatusCode: http.StatusFound,
					Header:     http.Header{"Location": {"http://93.184.216.34/loop"}},
					Proto:      "HTTP/1.1",
					ProtoMajor: 1,
					ProtoMinor: 1,
				}
				writeErr := resp.Write(conn)
				inner.Body.Close()
				if writeErr != nil {
					return
				}
			}
		}
		hits.Add(1)
		http.Redirect(w, r, "http://93.184.216.34/loop", http.StatusFound)
	}))
	defer proxy.Close()
	client := NewProxyManager(testProxyPools(proxy.URL)).GetHTTPClient(false)
	defer client.CloseIdleConnections()
	resp, err := client.Get("http://93.184.216.34/loop")
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "10 redirects") || hits.Load() != 10 {
		t.Fatalf("redirect limit lost: hits=%d, err=%v", hits.Load(), err)
	}
}
