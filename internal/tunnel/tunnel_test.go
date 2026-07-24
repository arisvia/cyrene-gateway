package tunnel

import "testing"

func TestParseAuthURL(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "standard auth url",
			input:  "To authenticate, visit:\n  https://login.tailscale.com/a/abc123def\n",
			expect: "https://login.tailscale.com/a/abc123def",
		},
		{
			name:   "no url",
			input:  "Success.\n",
			expect: "",
		},
		{
			name:   "url with trailing whitespace",
			input:  "https://login.tailscale.com/a/xyz789  \n",
			expect: "https://login.tailscale.com/a/xyz789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAuthURL(tt.input)
			if got != tt.expect {
				t.Errorf("parseAuthURL() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestParseEnableURL(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "enable url",
			input:  "Funnel is not enabled.\nEnable it at: https://login.tailscale.com/a/enable123\n",
			expect: "https://login.tailscale.com/a/enable123",
		},
		{
			name:   "no url",
			input:  "Funnel is not enabled.\n",
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEnableURL(tt.input)
			if got != tt.expect {
				t.Errorf("parseEnableURL() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestNewManager(t *testing.T) {
	m := NewManager("/tmp/test-data", 20128)
	if m.port != 20128 {
		t.Errorf("expected port 20128, got %d", m.port)
	}
	if m.dataDir != "/tmp/test-data" {
		t.Errorf("expected dataDir /tmp/test-data, got %s", m.dataDir)
	}

	status := m.GetStatus()
	if status.Platform == "" {
		t.Error("expected non-empty platform")
	}
}
