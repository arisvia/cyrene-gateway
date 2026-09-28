package provider

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Only local listeners are used; the proxy never forwards to a requested IP.
func startTunnelTestListener(t *testing.T, serve func(net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	conns := make(map[net.Conn]bool)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns[conn] = true
			mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				serve(conn)
			}()
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		mu.Lock()
		for conn := range conns {
			conn.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	return ln.Addr().String()
}

// Serve HTTP inside a local CONNECT without ever dialing the requested target.
// Existing policy/cache tests use this instead of an unsafe forward-proxy mock.
func serveTunnelTestHTTP(w http.ResponseWriter, r *http.Request, handler http.Handler, onClose func()) {
	if r.Method != http.MethodConnect {
		http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
		return
	}
	conn, rw, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	if onClose != nil {
		defer onClose()
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
	if rw.Flush() != nil {
		return
	}
	for {
		request, err := http.ReadRequest(rw.Reader)
		if err != nil {
			return
		}
		request.RemoteAddr = r.RemoteAddr
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		request.Body.Close()
		response := recorder.Result()
		err = response.Write(conn)
		response.Body.Close()
		if err != nil || request.Close || response.Close {
			return
		}
	}
}

func tunnelTestTLS(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "tunnel test"},
		DNSNames:  []string{"public.example", "proxy.example"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}, roots
}

func TestSafeTunnelPinsProxyWireAndPreservesAuthority(t *testing.T) {
	for _, proxyScheme := range []string{"http", "https", "socks5", "socks5h"} {
		for _, targetScheme := range []string{"http", "https"} {
			t.Run(proxyScheme+"/"+targetScheme, func(t *testing.T) {
				var targetQueries atomic.Int64
				stubTestDNSFunc(t, func(host string, kind uint16) []net.IP {
					if host == "proxy.example." {
						return []net.IP{net.ParseIP("127.0.0.1")}
					}
					if host == "public.example." && kind == 1 {
						if targetQueries.Add(1) == 1 {
							return []net.IP{net.ParseIP("93.184.216.34")}
						}
						return []net.IP{net.ParseIP("127.0.0.1")}
					}
					return nil
				})
				tlsConfig, roots := tunnelTestTLS(t)
				wantPort := 80
				if targetScheme == "https" {
					wantPort = 443
				}
				seen := make(chan string, 1)
				addr := startTunnelTestListener(t, func(conn net.Conn) {
					if proxyScheme == "https" {
						secured := tls.Server(conn, tlsConfig)
						if err := secured.Handshake(); err != nil {
							t.Error(err)
							return
						}
						if got := secured.ConnectionState().ServerName; got != "proxy.example" {
							t.Errorf("proxy SNI = %q", got)
						}
						conn = secured
					}
					if strings.HasPrefix(proxyScheme, "socks") {
						reader := bufio.NewReader(conn)
						var greeting [2]byte
						if _, err := io.ReadFull(reader, greeting[:]); err != nil {
							t.Error(err)
							return
						}
						methods := make([]byte, greeting[1])
						_, _ = io.ReadFull(reader, methods)
						_, _ = conn.Write([]byte{5, 0})
						var request [4]byte
						if _, err := io.ReadFull(reader, request[:]); err != nil {
							t.Error(err)
							return
						}
						if request != [4]byte{5, 1, 0, 1} {
							t.Errorf("SOCKS must send IPv4, not a hostname: %v", request)
							return
						}
						var target [6]byte
						_, _ = io.ReadFull(reader, target[:])
						if net.IP(target[:4]).String() != "93.184.216.34" || int(binary.BigEndian.Uint16(target[4:])) != wantPort {
							t.Errorf("SOCKS target = %v", target)
							return
						}
						_, _ = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
					} else {
						r, err := http.ReadRequest(bufio.NewReader(conn))
						if err != nil {
							t.Error(err)
							return
						}
						if r.Method != "CONNECT" || r.RequestURI != fmt.Sprintf("93.184.216.34:%d", wantPort) {
							t.Errorf("proxy received unsafe request: %s %s", r.Method, r.RequestURI)
							return
						}
						if got := r.Header.Get("Proxy-Authorization"); got != "Basic dXNlcjpwYXNz" {
							t.Errorf("proxy auth = %q", got)
						}
						_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
					}
					if targetScheme == "https" {
						secured := tls.Server(conn, tlsConfig)
						if err := secured.Handshake(); err != nil {
							t.Error(err)
							return
						}
						if got := secured.ConnectionState().ServerName; got != "public.example" {
							t.Errorf("target SNI = %q", got)
						}
						conn = secured
					}
					r, err := http.ReadRequest(bufio.NewReader(conn))
					if err != nil {
						t.Error(err)
						return
					}
					seen <- r.Host
					if r.RequestURI != "/path?q=1" || r.Header.Get("Proxy-Authorization") != "" {
						t.Errorf("origin request URI/auth = %q / %q", r.RequestURI, r.Header.Get("Proxy-Authorization"))
					}
					_, _ = io.WriteString(conn, "HTTP/1.1 204 No Content\r\nConnection: close\r\n\r\n")
				})
				_, port, _ := net.SplitHostPort(addr)
				client := NewProxyManager(testProxyPools(proxyScheme + "://user:pass@proxy.example:" + port)).GetHTTPClient(false)
				client.Transport.(*safeTransport).transport.TLSClientConfig = &tls.Config{RootCAs: roots}
				client.Timeout = 3 * time.Second
				defer client.CloseIdleConnections()
				resp, err := client.Get(targetScheme + "://public.example/path?q=1")
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
				if got := <-seen; got != "public.example" {
					t.Errorf("origin Host = %q", got)
				}
				if n := targetQueries.Load(); n != 1 {
					t.Errorf("target resolved %d times; dial must use validated IP", n)
				}
			})
		}
	}
}

func TestSafeTunnelProxyDNSCanChange(t *testing.T) {
	var phase atomic.Bool
	stubTestDNSFunc(t, func(host string, _ uint16) []net.IP {
		if host == "proxy.example." {
			if phase.Load() {
				return []net.IP{net.ParseIP("127.0.0.1")}
			}
			return []net.IP{net.ParseIP("127.0.0.2")}
		}
		return nil
	})
	addr := startTunnelTestListener(t, func(conn net.Conn) {
		r, err := http.ReadRequest(bufio.NewReader(conn))
		if err != nil {
			t.Error(err)
			return
		}
		if r.Method != "CONNECT" {
			t.Errorf("expected CONNECT, got %s", r.Method)
			return
		}
		_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\n\r\n")
		if _, err := http.ReadRequest(bufio.NewReader(conn)); err == nil {
			_, _ = io.WriteString(conn, "HTTP/1.1 204 No Content\r\nConnection: close\r\n\r\n")
		}
	})
	_, port, _ := net.SplitHostPort(addr)
	pm := NewProxyManager(testProxyPools("http://proxy.example:" + port))
	client := pm.GetHTTPClient(false)
	defer client.CloseIdleConnections()
	// Force a lookup before the change: construction must not freeze exemptions.
	if _, err := net.DefaultResolver.LookupIPAddr(context.Background(), "proxy.example"); err != nil {
		t.Fatal(err)
	}
	phase.Store(true)
	for range 2 {
		next := pm.GetHTTPClient(false)
		if next.Transport != client.Transport {
			t.Fatal("route transport was not reused")
		}
		resp, err := next.Get("http://93.184.216.34/")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
}

func TestSafeTunnelNoProxyHasNoEndpointExemption(t *testing.T) {
	if os.Getenv("CYRENE_TEST_NO_PROXY") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSafeTunnelNoProxyHasNoEndpointExemption$", "-test.v")
		cmd.Env = append(os.Environ(), "CYRENE_TEST_NO_PROXY=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("NO_PROXY subprocess: %v\n%s", err, out)
		}
		return
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer server.Close()
	for _, key := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"} {
		t.Setenv(key, server.URL)
	}
	t.Setenv("NO_PROXY", "public.example")
	t.Setenv("no_proxy", "public.example")
	t.Setenv("REQUEST_METHOD", "")
	client := SafeHTTPClient(time.Second, false)
	defer client.CloseIdleConnections()
	transport := client.Transport.(*safeTransport).transport
	req, _ := http.NewRequest("GET", "http://public.example/", nil)
	selected, err := http.ProxyFromEnvironment(req)
	if err != nil || selected != nil {
		t.Fatalf("expected NO_PROXY direct route, got %v, %v", selected, err)
	}
	// Exercise the actual dial boundary with the address a rebound hostname
	// would yield. An environment proxy's IP must never authorize this dial.
	u, _ := url.Parse(server.URL)
	conn, err := transport.DialContext(context.Background(), "tcp", u.Host)
	if conn != nil {
		conn.Close()
	}
	if err == nil {
		t.Fatal("NO_PROXY target's actual dial borrowed the proxy endpoint exemption")
	}
}

func TestSafeTunnelFallsBackAfterStalledCONNECT(t *testing.T) {
	seen := make(chan string, 2)
	firstClosed := make(chan error, 1)
	addr := startTunnelTestListener(t, func(conn net.Conn) {
		reader := bufio.NewReader(conn)
		r, err := http.ReadRequest(reader)
		if err != nil {
			t.Error(err)
			return
		}
		defer r.Body.Close()
		if r.Method != http.MethodConnect {
			t.Errorf("expected CONNECT, got %s", r.Method)
			return
		}
		seen <- r.RequestURI
		switch r.RequestURI {
		case "93.184.216.34:443":
			_, err := io.Copy(io.Discard, reader)
			firstClosed <- err
		case "93.184.216.35:443":
			if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
				t.Error(err)
				return
			}
			var ping [4]byte
			if _, err := io.ReadFull(reader, ping[:]); err != nil {
				t.Error(err)
				return
			}
			if string(ping[:]) != "ping" {
				t.Errorf("tunnel payload = %q", ping)
				return
			}
			if _, err := io.WriteString(conn, "pong"); err != nil {
				t.Error(err)
			}
		default:
			t.Errorf("unexpected CONNECT target: %s", r.RequestURI)
		}
	})
	proxyURL, err := url.Parse("http://" + addr)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	plan := &safeDialPlan{
		requestContext: ctx,
		addresses:      []string{"93.184.216.34:443", "93.184.216.35:443"},
		proxy:          proxyURL,
	}
	transport := newSafeTransport(false, proxyURL)
	defer transport.CloseIdleConnections()
	dialCtx := context.WithValue(context.WithoutCancel(ctx), safeDialPlanKey{}, plan)
	conn, err := transport.dialContext(dialCtx, "tcp", "public.example:443")
	if err != nil {
		t.Fatalf("reachable second address was starved by stalled CONNECT: %v", err)
	}
	defer conn.Close()
	for _, want := range plan.addresses {
		select {
		case got := <-seen:
			if got != want {
				t.Errorf("CONNECT target = %q, want %q", got, want)
			}
		default:
			t.Fatalf("missing CONNECT to %s", want)
		}
	}
	select {
	case err := <-firstClosed:
		if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
			t.Errorf("stalled connection leaked until server timeout: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stalled CONNECT connection was not closed")
	}
	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(conn, "ping"); err != nil {
		t.Fatalf("successful tunnel did not survive attempt cancellation: %v", err)
	}
	var pong [4]byte
	if _, err := io.ReadFull(conn, pong[:]); err != nil {
		t.Fatalf("read through successful tunnel: %v", err)
	}
	if string(pong[:]) != "pong" {
		t.Errorf("tunnel response = %q", pong)
	}
}

func TestSafeTunnelAlreadyCanceledRequest(t *testing.T) {
	addr := startTunnelTestListener(t, func(net.Conn) {
		t.Error("already-canceled request connected to proxy")
	})
	proxyURL, err := url.Parse("http://" + addr)
	if err != nil {
		t.Fatal(err)
	}
	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	plan := &safeDialPlan{
		requestContext: requestCtx,
		addresses:      []string{"93.184.216.34:443", "93.184.216.35:443"},
		proxy:          proxyURL,
	}
	ctx, cancelDial := context.WithTimeout(context.Background(), time.Second)
	defer cancelDial()
	transport := newSafeTransport(false, proxyURL)
	defer transport.CloseIdleConnections()
	conn, err := transport.dialContext(context.WithValue(ctx, safeDialPlanKey{}, plan), "tcp", "public.example:443")
	if conn != nil {
		conn.Close()
		t.Error("already-canceled request returned a connection")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected original request cancellation, got %v", err)
	}
}

func TestSafeTunnelCancelAndRejectCloseConnections(t *testing.T) {
	for _, mode := range []string{"http-cancel", "http-reject", "socks5-cancel", "socks5-reject", "https-cancel"} {
		t.Run(mode, func(t *testing.T) {
			accepted, closed := make(chan struct{}), make(chan error, 1)
			addr := startTunnelTestListener(t, func(conn net.Conn) {
				reader := bufio.NewReader(conn)
				if strings.HasPrefix(mode, "http-") {
					r, err := http.ReadRequest(reader)
					if err != nil {
						closed <- err
						return
					}
					if r.Method != "CONNECT" {
						t.Errorf("expected CONNECT, got %s", r.Method)
					}
					if mode == "http-reject" {
						_, _ = io.WriteString(conn, "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n")
					}
				} else {
					var first [2]byte
					_, _ = io.ReadFull(reader, first[:])
					if strings.HasPrefix(mode, "socks5") {
						_, _ = io.CopyN(io.Discard, reader, int64(first[1]))
						if mode == "socks5-reject" {
							_, _ = conn.Write([]byte{5, 255})
						}
					}
				}
				close(accepted)
				_, err := io.Copy(io.Discard, reader)
				closed <- err
			})
			scheme := strings.Split(mode, "-")[0]
			client := NewProxyManager(testProxyPools(scheme + "://" + addr)).GetHTTPClient(false)
			defer client.CloseIdleConnections()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, "GET", "http://93.184.216.34/", nil)
			done := make(chan error, 1)
			go func() {
				resp, err := client.Do(req)
				if resp != nil {
					resp.Body.Close()
				}
				done <- err
			}()
			select {
			case <-accepted:
			case <-time.After(2 * time.Second):
				t.Fatal("proxy handshake not reached")
			}
			if strings.HasSuffix(mode, "cancel") {
				cancel()
			}
			select {
			case err := <-done:
				if err == nil || (strings.HasSuffix(mode, "cancel") && !errors.Is(err, context.Canceled)) {
					t.Errorf("expected failure/cancel, got %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("request did not stop")
			}
			select {
			case err := <-closed:
				if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
					t.Errorf("connection leaked until server timeout: %v", err)
				}
			case <-time.After(time.Second):
				t.Error("failed/cancelled handshake leaked a connection")
			}
		})
	}
}
