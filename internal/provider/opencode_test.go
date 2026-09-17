package provider

import (
	"net/http"
	"strings"
	"testing"
)

func TestCanonicalSessionAndRequestIDs(t *testing.T) {
	for range 20 {
		ses := GenerateCanonicalSessionID()
		if !openCodeSessionRegex.MatchString(ses) {
			t.Errorf("expected session ID matching regex, got %q", ses)
		}
		if len(ses) != 30 {
			t.Errorf("expected session ID length 30, got %d (%q)", len(ses), ses)
		}

		msg := GenerateCanonicalRequestID()
		if !strings.HasPrefix(msg, "msg_") || len(msg) != 30 {
			t.Errorf("expected request ID length 30 with prefix msg_, got %q", msg)
		}
	}
}

func TestTranslateSessionID(t *testing.T) {
	canonical := "ses_f528155f3ffehzeD6kc5Wi4xp8"
	if got := TranslateSessionID(canonical); got != canonical {
		t.Errorf("expected valid canonical session preserved, got %q", got)
	}

	arbitrary := "claude:session-12345-uuid"
	translated := TranslateSessionID(arbitrary)
	if !openCodeSessionRegex.MatchString(translated) {
		t.Errorf("expected translated session to match canonical regex, got %q", translated)
	}
	if len(translated) != 30 {
		t.Errorf("expected translated session length 30, got %d", len(translated))
	}
}

func TestHasValidOpenCodeVersion(t *testing.T) {
	validCases := []string{
		"opencode/1.17.0",
		"opencode/1.18.31",
		"opencode/2.0.0",
		"OpenCode/1.18.0",
	}
	for _, ua := range validCases {
		if !hasValidOpenCodeVersion(ua) {
			t.Errorf("expected %q to be valid", ua)
		}
	}

	invalidCases := []string{
		"opencode",
		"opencode/1.16.5",
		"opencode/0.9.0",
		"Claude-Code/1.0",
		"",
	}
	for _, ua := range invalidCases {
		if hasValidOpenCodeVersion(ua) {
			t.Errorf("expected %q to be invalid", ua)
		}
	}
}

func TestIsOpenCodeFreeModel(t *testing.T) {
	freeModels := []string{
		"big-pickle",
		"opencode/big-pickle",
		"mimo-v2.5-free",
		"ling-3.0-flash-fin-free",
		"muse-spark-1.3-contributor-free",
		"union-alpha",
	}
	for _, m := range freeModels {
		if !IsOpenCodeFreeModel(m) {
			t.Errorf("expected %q to be recognized as free model", m)
		}
	}

	paidModels := []string{
		"claude-3-5-sonnet",
		"deepseek-v4-pro",
		"gpt-4o",
		"glm-5.3",
	}
	for _, m := range paidModels {
		if IsOpenCodeFreeModel(m) {
			t.Errorf("expected %q NOT to be recognized as free model", m)
		}
	}
}

func TestOpenCodeHeadersHook(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://opencode.ai/zen/v1/chat/completions", nil)
	creds := Credentials{}
	opencodeHeadersHook(req.Header, creds)

	if got := req.Header.Get("User-Agent"); got != DefaultOpenCodeUA {
		t.Errorf("expected User-Agent %q, got %q", DefaultOpenCodeUA, got)
	}
	if got := req.Header.Get("x-opencode-client"); got != "desktop" {
		t.Errorf("expected x-opencode-client desktop, got %q", got)
	}
	if got := req.Header.Get("x-opencode-session"); !openCodeSessionRegex.MatchString(got) {
		t.Errorf("expected canonical x-opencode-session, got %q", got)
	}
	if got := req.Header.Get("x-opencode-request"); !strings.HasPrefix(got, "msg_") || len(got) != 30 {
		t.Errorf("expected canonical x-opencode-request, got %q", got)
	}
}
