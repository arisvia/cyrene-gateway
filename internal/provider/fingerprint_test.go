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
func TestAggregatorCLIIdentity(t *testing.T) {
	if got := ClineAccessToken("eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1In0.sig"); got != "workos:eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1In0.sig" {
		t.Errorf("OAuth JWT must gain the workos: prefix, got %q", got)
	}
	if got := ClineAccessToken("clp_abc123"); got != "clp_abc123" {
		t.Errorf("ClinePass API keys must pass through verbatim, got %q", got)
	}
	if got := ClineAccessToken("workos:eyJhbGciOiJIUzI1NiJ9.e30.sig"); got != "workos:eyJhbGciOiJIUzI1NiJ9.e30.sig" {
		t.Errorf("already-prefixed tokens must not be prefixed twice, got %q", got)
	}

	cline := Registry["cline"]
	if cline.ModelsURL == "" || cline.ModelsAuth != "none" {
		t.Errorf("cline needs a public live catalog, got url=%q auth=%q", cline.ModelsURL, cline.ModelsAuth)
	}
	if len(cline.Models) != 0 {
		t.Error("cline must not hardcode a static model list")
	}

	kilo := Registry["kilocode"]
	if kilo.ModelsURL == "" || len(kilo.Models) != 0 {
		t.Error("kilocode needs a live catalog and no static model list")
	}
	if got := GetProviderFlowType("kilocode"); got != FlowDeviceCode {
		t.Errorf("kilocode flow = %q, want device_code", got)
	}
	if got := GetProviderFlowType("cline"); got != FlowAuthorizationCode {
		t.Errorf("cline flow = %q, want authorization_code", got)
	}
}

func TestKilocodeOrgHook(t *testing.T) {
	h := http.Header{}
	kilocodeOrgHook(h, Credentials{ProviderSpecificData: map[string]any{"orgId": "org-9"}})
	if got := h.Get("X-Kilocode-OrganizationID"); got != "org-9" {
		t.Errorf("org header = %q, want org-9", got)
	}
	// Single-org accounts have no orgId: the header must stay absent rather than
	// being sent empty.
	h2 := http.Header{}
	kilocodeOrgHook(h2, Credentials{})
	if got := h2.Get("X-Kilocode-OrganizationID"); got != "" {
		t.Errorf("org header must be omitted without an org, got %q", got)
	}
}
