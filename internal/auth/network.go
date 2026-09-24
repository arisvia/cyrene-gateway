package auth

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// IsLocalOrPrivateIP reports whether ip is a loopback, private LAN (RFC 1918/4193),
// link-local unicast (RFC 3927/4291), or CGNAT (RFC 6598/Tailscale) address.
// Unspecified and multicast addresses cannot establish a local client identity.
func IsLocalOrPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
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
	if strings.HasPrefix(h, "[") && strings.HasSuffix(h, "]") {
		return IsLocalOrPrivateIP(net.ParseIP(h[1 : len(h)-1]))
	}
	lower := strings.TrimSuffix(strings.ToLower(h), ".")

	if ip := net.ParseIP(lower); ip != nil {
		return IsLocalOrPrivateIP(ip)
	}

	// Only well-formed hostnames may qualify as local; malformed/missing hosts
	// must not become single-label LAN names by accident.
	for _, label := range strings.Split(lower, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
				return false
			}
		}
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

// ExtractEffectiveClientIP walks X-Forwarded-For from the TCP peer outwards,
// stopping at the first non-local hop. Headers never replace a public peer or
// public XFF hop with a private address. Private peers are not authenticated
// proxies: extra client-IP headers may only strengthen the login requirement.
// A nil result means the source cannot be established and must require login.
func ExtractEffectiveClientIP(r *http.Request) net.IP {
	if r == nil {
		return nil
	}
	peerIP := net.ParseIP(ClientIP(r.RemoteAddr))
	if !IsLocalOrPrivateIP(peerIP) {
		return peerIP
	}

	clientIP := peerIP
	if values := r.Header.Values("X-Forwarded-For"); len(values) > 0 {
		parts := strings.Split(strings.Join(values, ","), ",")
		for i := len(parts) - 1; i >= 0; i-- {
			clientIP = net.ParseIP(strings.TrimSpace(parts[i]))
			if !IsLocalOrPrivateIP(clientIP) {
				return clientIP
			}
		}
	}
	for _, header := range []string{"X-Real-IP", "CF-Connecting-IP"} {
		for _, value := range r.Header.Values(header) {
			ip := net.ParseIP(strings.TrimSpace(value))
			if !IsLocalOrPrivateIP(ip) {
				return ip
			}
		}
	}
	return clientIP
}

func isLocalRequestHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return IsLocalOrPrivateHost(host)
}

// IsPublicWANRequest reports whether management authentication is required by
// network evidence. Public or unknown sources, hosts and origins fail closed.
// Forwarded hosts supplement the original Host; they never replace it.
func IsPublicWANRequest(r *http.Request) bool {
	if r == nil || !IsLocalOrPrivateIP(ExtractEffectiveClientIP(r)) {
		return true
	}
	if !isLocalRequestHost(r.Host) {
		return true
	}
	for _, value := range r.Header.Values("X-Forwarded-Host") {
		for _, host := range strings.Split(value, ",") {
			if !isLocalRequestHost(strings.TrimSpace(host)) {
				return true
			}
		}
	}
	for _, origin := range r.Header.Values("Origin") {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !isLocalRequestHost(u.Host) {
			return true
		}
	}
	return false
}
