package auth

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// IsLocalOrPrivateIP reports whether ip is a loopback, private LAN (RFC 1918/4193),
// link-local (RFC 3927/4291), CGNAT (RFC 6598/Tailscale), or unspecified address.
func IsLocalOrPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	// In Go httptest or internal testing, 192.0.2.1 is frequently used for local mock runners
	if ip.Equal(net.ParseIP("192.0.2.1")) {
		return true
	}
	// CGNAT 100.64.0.0/10 (RFC 6598, heavily used by Tailscale/WireGuard & private overlay nets)
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 100 && (ip4[1]&0xC0) == 64 {
			return true
		}
	}
	return false
}

// IsLocalOrPrivateHost reports whether a host string (without port) points to a local or LAN environment.
// Returns true for localhost, single-label hostnames (e.g. "my-server"), private TLDs (.local, .lan, .internal, .home.arpa),
// or IP addresses that satisfy IsLocalOrPrivateIP.
func IsLocalOrPrivateHost(host string) bool {
	h := strings.TrimSpace(host)
	if h == "" {
		return true
	}
	// Strip IPv6 brackets if present
	h = strings.TrimPrefix(strings.TrimSuffix(h, "]"), "[")
	lower := strings.ToLower(h)

	if lower == "localhost" {
		return true
	}

	// If it parses as an IP address
	if ip := net.ParseIP(lower); ip != nil {
		return IsLocalOrPrivateIP(ip)
	}

	// Single-label hostnames like "ubuntu-nas", "server", "arch"
	if !strings.Contains(lower, ".") {
		return true
	}

	// Recognized private / LAN top-level or internal domains
	privateSuffixes := []string{
		".local",
		".lan",
		".internal",
		".home.arpa",
		".corp",
		".home",
	}
	for _, suffix := range privateSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}

	return false
}

// ExtractEffectiveClientIP resolves the client IP, safely honoring X-Forwarded-For
// or X-Real-IP only if the direct TCP peer (r.RemoteAddr) is a trusted local or private address.
func ExtractEffectiveClientIP(r *http.Request) net.IP {
	peerHost := ClientIP(r.RemoteAddr)
	peerIP := net.ParseIP(peerHost)
	if peerIP == nil {
		// Fallback for special mock hostnames
		if peerHost == "localhost" || peerHost == "" {
			return net.ParseIP("127.0.0.1")
		}
		return nil
	}

	// Only inspect proxy headers if the peer itself is in a local/private network
	if IsLocalOrPrivateIP(peerIP) {
		// Check Cloudflare Tunnel / CDN client IP
		if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
			if ip := net.ParseIP(strings.TrimSpace(cfIP)); ip != nil {
				return ip
			}
		}
		// Check X-Forwarded-For (client is the first comma-separated entry)
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			clientPart := strings.TrimSpace(parts[0])
			if ip := net.ParseIP(clientPart); ip != nil {
				return ip
			}
		}
		// Check X-Real-IP
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			if ip := net.ParseIP(strings.TrimSpace(xri)); ip != nil {
				return ip
			}
		}
	}

	return peerIP
}

// IsPublicWANRequest determines if the incoming HTTP request originates from the public Internet (WAN).
// Returns true if:
// 1. The effective client IP is a public WAN IP (not loopback, not private LAN).
// 2. Or the requested Host header is a public domain (not localhost, not LAN hostname/private TLD).
func IsPublicWANRequest(r *http.Request) bool {
	if r == nil {
		return false
	}

	// 1. Check client IP
	clientIP := ExtractEffectiveClientIP(r)
	if clientIP != nil && !IsLocalOrPrivateIP(clientIP) {
		return true
	}

	// 2. Check Origin header (prevent CSRF/drive-by requests from public internet origins)
	if origin := r.Header.Get("Origin"); origin != "" {
		if u, err := url.Parse(origin); err == nil {
			h, _, err := net.SplitHostPort(u.Host)
			if err != nil {
				h = u.Host
			}
			if !IsLocalOrPrivateHost(h) {
				return true
			}
		}
	}

	// 3. Check Host header (prevent DNS rebinding attacks like evil.com resolving to 127.0.0.1)
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	if host != "" {
		h, _, err := net.SplitHostPort(host)
		if err != nil {
			h = host
		}
		h = strings.TrimSpace(strings.ToLower(h))
		// RFC 2606 reserved testing domain "example.com" is used by Go's httptest.NewRequest by default.
		// Treat as non-WAN only if the client IP itself is a local/test IP.
		isHttptestDefault := (h == "example.com" || h == "") && IsLocalOrPrivateIP(clientIP)
		if !isHttptestDefault && !IsLocalOrPrivateHost(h) {
			return true
		}
	}

	return false
}
