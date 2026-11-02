package provider

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// Upstream client-check gates read these headers. A value that names this
// gateway (rather than the official CLI) or omits the header entirely is what
// marks the traffic as relayed, so pin the wire-visible identity here.
func TestClientFingerprintsMatchOfficialCLIs(t *testing.T) {
	prepare := func(providerID, modelID string) *http.Request {
		t.Helper()
		conn := &model.ProviderConnection{
			ID: "c", Provider: providerID, AuthType: "oauth",
			Data: model.ConnectionData{AccessToken: "tok"},
		}
		req, _, err := PrepareUpstreamRequest(context.Background(),
			ExecutionRequest{RawBody: []byte(`{"messages":[{"role":"user","content":"hi"}]}`)},
			ExecutionCandidate{
				Connection:   conn,
				ModelInfo:    model.ModelInfo{Provider: providerID, Model: modelID},
				ProviderInfo: Registry[providerID],
			}, nil)
		if err != nil {
			t.Fatalf("%s: %v", providerID, err)
		}
		return req
	}

	t.Run("kimi", func(t *testing.T) {
		req := prepare("kimi", "kimi-for-coding")
		// The coding backend 403s unless the UA is KimiCLI/<version>.
		if got := req.Header.Get("User-Agent"); got != KimiCLIUA {
			t.Errorf("User-Agent = %q, want %q", got, KimiCLIUA)
		}
		if got := req.Header.Get("X-Msh-Platform"); got != "kimi_cli" {
			t.Errorf("X-Msh-Platform = %q, want kimi_cli", got)
		}
		if got := req.Header.Get("X-Msh-Version"); got != KimiCLIVersion {
			t.Errorf("X-Msh-Version = %q, want %q", got, KimiCLIVersion)
		}
	})

	t.Run("grok-cli", func(t *testing.T) {
		req := prepare("grok-cli", GrokCLIModel)
		if got := req.URL.String(); got != GrokCLIBaseURL+"/responses" {
			t.Errorf("upstream URL = %q, want %q", got, GrokCLIBaseURL+"/responses")
		}
		if got := req.Header.Get("x-grok-client-version"); got != GrokCLIVersion {
			t.Errorf("x-grok-client-version = %q, want %q", got, GrokCLIVersion)
		}
		if got := req.Header.Get("x-grok-client-identifier"); got != GrokCLIClientIdentifier {
			t.Errorf("x-grok-client-identifier = %q, want %q", got, GrokCLIClientIdentifier)
		}
		// cli-chat-proxy keys conversation state off these three.
		for _, h := range []string{"x-grok-session-id", "x-grok-conv-id", "x-grok-req-id"} {
			if req.Header.Get(h) == "" {
				t.Errorf("%s must be set", h)
			}
		}
		if got := req.Header.Get("x-grok-conv-id"); got != req.Header.Get("x-grok-session-id") {
			t.Errorf("conv-id %q must mirror session-id %q", got, req.Header.Get("x-grok-session-id"))
		}
	})
}

// Qoder signs with COSY headers, but the chat path must still name the CLI in
// User-Agent: an absent UA makes Go send "Go-http-client/2.0", which tells the
// upstream gateway the request came from a relay.
func TestQoderIdentityConstantsAreClientShaped(t *testing.T) {
	if QoderCLIUserAgent == "" || !strings.HasPrefix(QoderCLIUserAgent, "qodercli/") {
		t.Errorf("Qoder User-Agent must look like the CLI, got %q", QoderCLIUserAgent)
	}
	if QoderIDEVersion == "" {
		t.Error("QoderIDEVersion must be set")
	}
}

// Aggregator CLIs front a plain OpenAI-compatible endpoint, so only the
// credential shape and identity headers need pinning.
