package provider

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

var (
	ErrPrivateNetworkBlocked = errors.New("access to private, loopback, or link-local address is blocked by SSRF policy")
	ErrUnsupportedScheme     = errors.New("url scheme must be https (or http for localhost in dev)")
)

// ValidateUpstreamURL validates a user-provided BaseURL or PanelURL against SSRF risks.
func ValidateUpstreamURL(rawURL string, allowPrivate bool) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, ErrUnsupportedScheme
	}

	host := u.Hostname()
	if host == "" {
		return nil, errors.New("empty host")
	}

	if !allowPrivate {
		lowerHost := strings.ToLower(host)
		if lowerHost == "localhost" ||
			strings.HasSuffix(lowerHost, ".local") ||
			strings.HasSuffix(lowerHost, ".internal") ||
			strings.HasSuffix(lowerHost, ".lan") ||
			strings.HasSuffix(lowerHost, ".home.arpa") {
			return nil, fmt.Errorf("%w: reserved local domain %s", ErrPrivateNetworkBlocked, host)
		}

		// Direct IP check
		if ip := net.ParseIP(host); ip != nil {
			if isBlockedIP(ip) {
				return nil, fmt.Errorf("%w: resolved to %s", ErrPrivateNetworkBlocked, ip.String())
			}
			return u, nil
		}

		// Domain resolution check (if network is available)
		ips, err := net.LookupIP(host)
		if err == nil {
			for _, ip := range ips {
				if isBlockedIP(ip) {
					return nil, fmt.Errorf("%w: resolved to %s", ErrPrivateNetworkBlocked, ip.String())
				}
			}
		}
	}

	return u, nil
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	// Explicitly check IPv4 cloud metadata range 169.254.0.0/16 and CGNAT 100.64.0.0/10
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

// SafeDialerControl returns a Control function for net.Dialer that blocks SSRF targets at dial time.
func SafeDialerControl(allowPrivate bool) func(network, address string, c syscall.RawConn) error {
	return dialerControlExempt(allowPrivate, nil)
}

// dialerControlExempt is SafeDialerControl with a permit-list of hosts/IPs that
// are always dialable (the operator's outbound proxy endpoints).
func dialerControlExempt(allowPrivate bool, exempt map[string]bool) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		if allowPrivate {
			return nil
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}
		if exempt[host] {
			return nil
		}
		ip := net.ParseIP(host)
		if ip != nil && isBlockedIP(ip) {
			return ErrPrivateNetworkBlocked
		}
		return nil
	}
}

func safeDialer(allowPrivate bool, exempt map[string]bool) *net.Dialer {
	return &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   dialerControlExempt(allowPrivate, exempt),
	}
}

// SafeHTTPClient returns an HTTP client equipped with custom dialer checking every resolved IP against SSRF rules.
func SafeHTTPClient(timeout time.Duration, allowPrivate bool) *http.Client {
	dialer := safeDialer(allowPrivate, nil)

	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			if !allowPrivate {
				_, err := ValidateUpstreamURL(req.URL.String(), false)
				if err != nil {
					return fmt.Errorf("redirect blocked: %w", err)
				}
			}
			return nil
		},
	}
}
