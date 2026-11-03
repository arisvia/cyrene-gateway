package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

// A client secret is a credential. It must never cross the /api/registry
// boundary, where it would be readable by any caller when login is optional.
func TestRegistryJSONNeverCarriesClientSecret(t *testing.T) {
	for id, p := range Registry {
		if p.ClientSecret == "" {
			continue
		}
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("%s: marshal: %v", id, err)
		}
		body := string(raw)
		if strings.Contains(body, p.ClientSecret) {
			t.Errorf("%s: registry JSON leaks the client secret", id)
		}
		if strings.Contains(strings.ToLower(body), "clientsecret") {
			t.Errorf("%s: clientSecret key present in registry JSON: %s", id, body)
		}
	}
}
