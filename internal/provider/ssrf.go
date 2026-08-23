package provider

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
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
		// Resolve IP to check for loopback, private, link-local, multicast
		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve host %q: %w", host, err)
		}
		for _, ip := range ips {
			if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
				return nil, fmt.Errorf("%w: resolved to %s", ErrPrivateNetworkBlocked, ip.String())
			}
		}
	}

	return u, nil
}

// SafeHTTPClient returns an HTTP client equipped with custom dialer checking every resolved IP against SSRF rules.
func SafeHTTPClient(timeout time.Duration, allowPrivate bool) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			if allowPrivate {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				host = address
			}
			ip := net.ParseIP(host)
			if ip != nil {
				if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
					return ErrPrivateNetworkBlocked
				}
			}
			return nil
		},
	}

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
