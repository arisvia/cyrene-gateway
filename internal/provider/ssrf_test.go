package provider

import (
	"testing"
)

func TestValidateUpstreamURLSSRF(t *testing.T) {
	tests := []struct {
		url          string
		allowPrivate bool
		wantErr      bool
	}{
		{"https://api.openai.com/v1", false, false},
		{"https://127.0.0.1:8080", false, true},
		{"http://localhost:3000", false, true},
		{"https://192.168.1.1/v1", false, true},
		{"https://10.0.0.1/v1", false, true},
		{"https://169.254.169.254/latest/meta-data", false, true},
		{"ftp://api.openai.com", false, true},
		{"http://localhost:3000", true, false},
	}

	for _, tt := range tests {
		_, err := ValidateUpstreamURL(tt.url, tt.allowPrivate)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateUpstreamURL(%q, %v) error = %v, wantErr %v", tt.url, tt.allowPrivate, err, tt.wantErr)
		}
	}
}
