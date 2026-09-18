package auth

import (
	"net"
	"net/http/httptest"
	"testing"
)

func TestIsLocalOrPrivateIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"10.0.0.1", true},
		{"10.254.1.99", true},
		{"172.16.0.1", true},
		{"172.31.255.254", true},
		{"192.168.1.1", true},
		{"192.168.100.254", true},
		{"169.254.1.2", true},
		{"100.64.0.1", true},      // Tailscale/CGNAT
		{"100.100.100.100", true}, // Tailscale/CGNAT
		{"192.0.2.1", true},       // mock test runner
		{"0.0.0.0", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"203.0.113.195", false},
		{"142.250.190.46", false},
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if got := IsLocalOrPrivateIP(ip); got != tt.want {
			t.Errorf("IsLocalOrPrivateIP(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestIsLocalOrPrivateHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"192.168.1.100", true},
		{"10.0.0.5", true},
		{"my-server", true},
		{"ubuntu-nas", true},
		{"home-gateway", true},
		{"cyrene.local", true},
		{"router.lan", true},
		{"node.internal", true},
		{"device.home.arpa", true},
		{"nas.home", true},
		{"example.com", false},
		{"api.cyrene.ai", false},
		{"203.0.113.5", false},
	}

	for _, tt := range tests {
		if got := IsLocalOrPrivateHost(tt.host); got != tt.want {
			t.Errorf("IsLocalOrPrivateHost(%s) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestIsPublicWANRequest(t *testing.T) {
	// 1. Local loopback request
	req1 := httptest.NewRequest("GET", "http://localhost:8080/api/settings", nil)
	req1.RemoteAddr = "127.0.0.1:12345"
	if IsPublicWANRequest(req1) {
		t.Errorf("expected localhost request not to be WAN")
	}

	// 2. LAN private request
	req2 := httptest.NewRequest("GET", "http://192.168.1.10:8080/api/settings", nil)
	req2.RemoteAddr = "192.168.1.50:54321"
	if IsPublicWANRequest(req2) {
		t.Errorf("expected LAN request not to be WAN")
	}

	// 3. Tailscale private request
	req3 := httptest.NewRequest("GET", "http://100.100.1.2:8080/api/settings", nil)
	req3.RemoteAddr = "100.64.0.5:54321"
	if IsPublicWANRequest(req3) {
		t.Errorf("expected Tailscale CGNAT request not to be WAN")
	}

	// 4. Public WAN direct IP request
	req4 := httptest.NewRequest("GET", "http://203.0.113.195:8080/api/settings", nil)
	req4.RemoteAddr = "203.0.113.55:54321"
	if !IsPublicWANRequest(req4) {
		t.Errorf("expected public IP request to be WAN")
	}

	// 5. Reverse proxy on LAN forwarding WAN client
	req5 := httptest.NewRequest("GET", "http://gateway.local/api/settings", nil)
	req5.RemoteAddr = "127.0.0.1:80" // local Nginx reverse proxy
	req5.Header.Set("X-Forwarded-For", "203.0.113.99, 10.0.0.1")
	if !IsPublicWANRequest(req5) {
		t.Errorf("expected WAN client forwarded by local proxy to be WAN")
	}

	// 6. Cross-origin request from public internet domain
	req6 := httptest.NewRequest("GET", "http://192.168.1.10:8080/api/settings", nil)
	req6.RemoteAddr = "192.168.1.50:12345"
	req6.Header.Set("Origin", "https://malicious-public-site.com")
	if !IsPublicWANRequest(req6) {
		t.Errorf("expected cross-origin request from public domain to be WAN")
	}
}

func TestCloudflareTunnelDetection(t *testing.T) {
	// cloudflared running locally on loopback forwarding request from public client
	req := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/settings", nil)
	req.RemoteAddr = "127.0.0.1:39102"
	req.Host = "gateway.mycustomdomain.com"
	req.Header.Set("CF-Connecting-IP", "203.0.113.88")
	req.Header.Set("CF-Visitor", `{"scheme":"https"}`)
	req.Header.Set("X-Forwarded-Proto", "https")

	clientIP := ExtractEffectiveClientIP(req)
	if clientIP == nil || clientIP.String() != "203.0.113.88" {
		t.Errorf("expected CF-Connecting-IP 203.0.113.88 to be extracted, got %v", clientIP)
	}

	if !IsPublicWANRequest(req) {
		t.Errorf("expected request through Cloudflare Tunnel to be identified as public WAN")
	}
}
