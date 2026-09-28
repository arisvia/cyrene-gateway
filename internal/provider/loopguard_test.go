package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/loopguard"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

// loopingRawBody trips the text-repeat detector via three identical assistant turns.
const loopingRawBody = `{"messages":[` +
	`{"role":"user","content":"summarize the repository"},` +
	`{"role":"assistant","content":"I will read the repository files before answering"},` +
	`{"role":"assistant","content":"I will read the repository files before answering"},` +
	`{"role":"assistant","content":"I will read the repository files before answering"}]}`

const plainRawBody = `{"messages":[{"role":"user","content":"summarize the repository"}]}`

const hintMarker = "STOP repeating yourself"

func decodeBody(t *testing.T, raw string) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func marshalBody(t *testing.T, body map[string]any) string {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestApplyLoopGuardLeavesBodyUntouchedWithoutLoopOrTools(t *testing.T) {
	body := decodeBody(t, plainRawBody)
	before := decodeBody(t, plainRawBody)

	ApplyLoopGuard(body, []byte(plainRawBody), "openai", false)

	if !reflect.DeepEqual(body, before) {
		t.Errorf("guard-free request was modified:\nbefore=%v\nafter =%v", before, body)
	}
}

func TestApplyLoopGuardOpenAIAppendsInDetectOrder(t *testing.T) {
	body := decodeBody(t, `{"messages":[`+
		`{"role":"system","content":"You are helpful."},`+
		`{"role":"user","content":"summarize the repository"}]}`)

	ApplyLoopGuard(body, []byte(loopingRawBody), "openai", true)

	sys := body["messages"].([]any)[0].(map[string]any)
	got, _ := sys["content"].(string)
	want := loopguard.DetectLoop(mustMessages(t, loopingRawBody)).Hint
	if got != "You are helpful.\n\n"+want+"\n\n"+loopguard.TerminationPrompt {
		t.Fatalf("unexpected composed system message: %q", got)
	}

	ApplyLoopGuard(body, []byte(loopingRawBody), "openai", true)
	reapplied, _ := body["messages"].([]any)[0].(map[string]any)["content"].(string)
	if n := strings.Count(reapplied, want); n != 1 {
		t.Errorf("loop hint duplicated: %d occurrences", n)
	}
	if n := strings.Count(reapplied, loopguard.TerminationPrompt); n != 1 {
		t.Errorf("termination contract duplicated: %d occurrences", n)
	}
}

func TestApplyLoopGuardOpenAIPrependsWithoutSystemMessage(t *testing.T) {
	body := decodeBody(t, plainRawBody)

	ApplyLoopGuard(body, []byte(loopingRawBody), "openai", false)

	msgs := body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected one prepended system message, got %d messages", len(msgs))
	}
	first := msgs[0].(map[string]any)
	hint := loopguard.DetectLoop(mustMessages(t, loopingRawBody)).Hint
	if first["role"] != "system" || first["content"] != hint {
		t.Errorf("unexpected prepended message: %+v", first)
	}
	if strings.Contains(marshalBody(t, body), loopguard.TerminationPrompt) {
		t.Error("termination contract must not appear when the request has no tools")
	}
}

func TestApplyLoopGuardOpenAIEmptySystemContentGetsContractAlone(t *testing.T) {
	body := decodeBody(t, `{"messages":[`+
		`{"role":"system","content":""},`+
		`{"role":"user","content":"hi"}]}`)

	ApplyLoopGuard(body, []byte(plainRawBody), "openai", true)

	content := body["messages"].([]any)[0].(map[string]any)["content"]
	if content != loopguard.TerminationPrompt {
		t.Errorf("an empty system message must receive the contract alone, got %v", content)
	}
	if strings.Contains(marshalBody(t, body), hintMarker) {
		t.Error("loop hint must not appear for a conversation without a loop")
	}
}

func TestApplyLoopGuardResponsesInputShape(t *testing.T) {
	body := decodeBody(t, `{"input":[`+
		`{"role":"developer","content":[{"type":"input_text","text":"base"}]}]}`)

	ApplyLoopGuard(body, []byte(plainRawBody), "openai", true)

	parts := body["input"].([]any)[0].(map[string]any)["content"].([]any)
	last := parts[len(parts)-1].(map[string]any)
	if last["text"] != loopguard.TerminationPrompt {
		t.Errorf("contract was not appended: %v", last["text"])
	}
	if last["type"] != "input_text" {
		t.Errorf("Responses input parts must use input_text, got %v", last["type"])
	}
}

func TestApplyLoopGuardAnthropicSystem(t *testing.T) {
	t.Run("string system", func(t *testing.T) {
		body := decodeBody(t, `{"system":"Base.","messages":[{"role":"user","content":"hi"}]}`)

		ApplyLoopGuard(body, []byte(loopingRawBody), "anthropic", true)

		sys, _ := body["system"].(string)
		if !strings.HasPrefix(sys, "Base.\n\n") {
			t.Errorf("existing system must be preserved: %q", sys)
		}
		if !strings.Contains(sys, loopguard.TerminationPrompt) || !strings.Contains(sys, hintMarker) {
			t.Errorf("anthropic system missing guard text: %q", sys)
		}
	})

	t.Run("block system", func(t *testing.T) {
		body := decodeBody(t, `{"system":[{"type":"text","text":"Base."}],"messages":[]}`)

		ApplyLoopGuard(body, []byte(plainRawBody), "anthropic", true)

		blocks := body["system"].([]any)
		last := blocks[len(blocks)-1].(map[string]any)
		if last["type"] != "text" || last["text"] != loopguard.TerminationPrompt {
			t.Errorf("unexpected appended system block: %+v", last)
		}
	})
}

func TestApplyLoopGuardGeminiSystemInstruction(t *testing.T) {
	t.Run("camel case", func(t *testing.T) {
		body := decodeBody(t, `{"contents":[{"role":"user","parts":[{"text":"hi"}]}],`+
			`"systemInstruction":{"parts":[{"text":"Base."}]}}`)

		ApplyLoopGuard(body, []byte(loopingRawBody), "gemini", true)

		parts := body["systemInstruction"].(map[string]any)["parts"].([]any)
		last := parts[len(parts)-1].(map[string]any)
		if last["text"] != loopguard.TerminationPrompt {
			t.Errorf("contract was not appended to systemInstruction: %v", last["text"])
		}
		if !strings.Contains(marshalBody(t, body), hintMarker) {
			t.Error("loop hint missing from gemini body")
		}
	})

	t.Run("snake case", func(t *testing.T) {
		body := decodeBody(t, `{"system_instruction":{"parts":[{"text":"Base."}]}}`)

		ApplyLoopGuard(body, []byte(plainRawBody), "gemini", true)

		if _, ok := body["systemInstruction"]; ok {
			t.Fatal("a snake_case body must not gain a camelCase twin")
		}
		parts := body["system_instruction"].(map[string]any)["parts"].([]any)
		last := parts[len(parts)-1].(map[string]any)
		if last["text"] != loopguard.TerminationPrompt {
			t.Errorf("contract missing from system_instruction: %v", last)
		}
	})

	t.Run("absent systemInstruction", func(t *testing.T) {
		body := decodeBody(t, `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)

		ApplyLoopGuard(body, []byte(plainRawBody), "gemini", true)

		sys, ok := body["systemInstruction"].(map[string]any)
		if !ok {
			t.Fatalf("gemini body must gain a systemInstruction, got %v", body)
		}
		parts := sys["parts"].([]any)
		if len(parts) != 1 || parts[0].(map[string]any)["text"] != loopguard.TerminationPrompt {
			t.Errorf("unexpected created systemInstruction: %+v", sys)
		}
	})
}

// The Qoder executor hoists system messages out of the array, so the guard text
// has to survive into the payload's system field.
func TestApplyLoopGuardFeedsQoderSystemField(t *testing.T) {
	body := decodeBody(t, loopingRawBody)

	ApplyLoopGuard(body, []byte(loopingRawBody), "openai", true)

	messages, systemText := qoderNormalizeMessages(body)
	for _, msg := range messages {
		if msg["role"] == "system" {
			t.Fatalf("system message leaked into the Qoder messages array: %v", msg)
		}
	}
	if !strings.Contains(systemText, loopguard.TerminationPrompt) || !strings.Contains(systemText, hintMarker) {
		t.Errorf("Qoder system field missing guard text: %q", systemText)
	}
}

func TestApplyLoopGuardDetectOrderWithoutTools(t *testing.T) {
	body := decodeBody(t, loopingRawBody)

	ApplyLoopGuard(body, []byte(loopingRawBody), "openai", false)

	encoded := marshalBody(t, body)
	if !strings.Contains(encoded, hintMarker) {
		t.Error("loop hint missing for a detected loop")
	}
	if strings.Contains(encoded, loopguard.TerminationPrompt) {
		t.Error("termination contract requires tools")
	}
}

// Provider sanitizers rewrite the system message, so the guard has to compose
// after them or its text never reaches the upstream.
func TestPrepareUpstreamRequestLoopGuardSurvivesProviderSanitizers(t *testing.T) {
	cand := ExecutionCandidate{
		Connection: &model.ProviderConnection{
			ID: "cb-guard", Provider: "codebuddy", AuthType: "oauth",
			Data: model.ConnectionData{AccessToken: "cb-test-token"},
		},
		ModelInfo:    model.ModelInfo{Provider: "codebuddy", Model: "hy4-preview"},
		ProviderInfo: Registry["codebuddy"],
	}

	req, _, err := PrepareUpstreamRequest(
		context.Background(),
		ExecutionRequest{RawBody: []byte(loopingRawBody), Stream: false, HasTools: true, LoopGuardEnabled: true},
		cand,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(loopguard.TerminationPrompt)) {
		t.Errorf("termination contract never reached the codebuddy body: %s", body)
	}
	if !bytes.Contains(body, []byte(hintMarker)) {
		t.Errorf("loop hint never reached the codebuddy body: %s", body)
	}
}

// Chat Completions traffic is re-serialised for Anthropic and Gemini upstreams,
// so the guard text has to survive that translation as well.
func TestPrepareUpstreamRequestLoopGuardAcrossTargetFormats(t *testing.T) {
	for _, tc := range []struct{ providerID, wantField string }{
		{"anthropic", "system"},
		{"gemini", "systemInstruction"},
	} {
		t.Run(tc.providerID, func(t *testing.T) {
			cand := ExecutionCandidate{
				Connection: &model.ProviderConnection{
					ID: tc.providerID + "-guard", Provider: tc.providerID, AuthType: "api-key",
					Data: model.ConnectionData{APIKey: "sk-test", BaseURL: "https://upstream.invalid/v1"},
				},
				ModelInfo:    model.ModelInfo{Provider: tc.providerID, Model: "test-model"},
				ModelStr:     tc.providerID + "/test-model",
				ProviderInfo: Registry[tc.providerID],
			}
			req, _, err := PrepareUpstreamRequest(context.Background(),
				ExecutionRequest{RawBody: []byte(loopingRawBody), Stream: false, HasTools: true, LoopGuardEnabled: true},
				cand, nil)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			var parsed map[string]any
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Fatalf("body is not json: %v\n%s", err, body)
			}
			if _, ok := parsed[tc.wantField]; !ok {
				t.Fatalf("the %s body lost its %q field: %s", tc.providerID, tc.wantField, body)
			}
			if !bytes.Contains(body, []byte(loopguard.TerminationPrompt)) {
				t.Errorf("termination contract lost while translating to %s: %s", tc.providerID, body)
			}
			if !bytes.Contains(body, []byte(hintMarker)) {
				t.Errorf("loop hint lost while translating to %s: %s", tc.providerID, body)
			}
		})
	}
}

func mustMessages(t *testing.T, raw string) []loopguard.Message {
	t.Helper()
	var history struct {
		Messages []loopguard.Message `json:"messages"`
	}
	if err := json.Unmarshal([]byte(raw), &history); err != nil {
		t.Fatal(err)
	}
	return history.Messages
}
