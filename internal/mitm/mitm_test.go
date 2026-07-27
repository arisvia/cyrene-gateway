package mitm

import (
	"crypto/tls"
	"testing"
)

func TestGetToolForHost(t *testing.T) {
	cases := map[string]string{
		"api.individual.githubcopilot.com":     "copilot",
		"api.individual.githubcopilot.com:443": "copilot",
		"cloudcode-pa.googleapis.com":          "antigravity",
		"daily-cloudcode-pa.googleapis.com":    "antigravity",
		"runtime.us-east-1.kiro.dev":           "kiro",
		"q.us-east-1.amazonaws.com":            "kiro",
		"api2.cursor.sh":                       "cursor",
		"example.com":                          "",
	}
	for host, want := range cases {
		if got := GetToolForHost(host); got != want {
			t.Errorf("GetToolForHost(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestAllToolHosts(t *testing.T) {
	hosts := AllToolHosts()
	if len(hosts) == 0 {
		t.Fatal("expected non-empty host list")
	}
	seen := make(map[string]bool)
	for _, h := range hosts {
		if seen[h] {
			t.Errorf("duplicate host %q", h)
		}
		seen[h] = true
	}
}

func TestCertManagerGenerateAndLeaf(t *testing.T) {
	dir := t.TempDir()
	cm := NewCertManager(dir)

	if err := cm.EnsureRootCA(); err != nil {
		t.Fatalf("EnsureRootCA: %v", err)
	}
	if !cm.CertExists() {
		t.Fatal("expected cert to exist after generation")
	}

	// Reload from disk should succeed (idempotent)
	cm2 := NewCertManager(dir)
	if err := cm2.EnsureRootCA(); err != nil {
		t.Fatalf("EnsureRootCA reload: %v", err)
	}

	// Generate a leaf cert via SNI callback
	hello := &tls.ClientHelloInfo{ServerName: "api2.cursor.sh"}
	leaf, err := cm.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if leaf == nil || len(leaf.Certificate) == 0 {
		t.Fatal("expected leaf certificate")
	}

	// Cached on second call
	leaf2, err := cm.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate cached: %v", err)
	}
	if leaf != leaf2 {
		t.Error("expected cached certificate to be identical pointer")
	}

	pem, err := cm.RootCACertPEM()
	if err != nil || len(pem) == 0 {
		t.Fatalf("RootCACertPEM: err=%v len=%d", err, len(pem))
	}
}

func TestExtractModel(t *testing.T) {
	if m := extractModel("/v1/models/gemini-2.0-flash:generateContent", nil); m != "gemini-2.0-flash" {
		t.Errorf("url model = %q", m)
	}
	body := []byte(`{"model":"gpt-4o","messages":[]}`)
	if m := extractModel("/chat/completions", body); m != "gpt-4o" {
		t.Errorf("body model = %q", m)
	}
	if m := extractModel("/foo", []byte(`not json`)); m != "" {
		t.Errorf("expected empty model, got %q", m)
	}
}
