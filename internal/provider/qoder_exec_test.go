package provider

import (
	"encoding/json"
	"testing"
)

func TestQoderSSEBillingDetectionPreservesCompletion(t *testing.T) {
	for _, body := range []string{
		`{"choices":[{"delta":{"content":"insufficient evidence of arrears"}}]}`,
		`{"choices":[{"delta":{"reasoning_content":"check pricingUrl first"}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"{\"code\":\"110\",\"pricingUrl\":\"test\"}"}}]}}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			envelope, err := json.Marshal(map[string]any{"statusCodeValue": 200, "body": body})
			if err != nil {
				t.Fatal(err)
			}
			data, done := UnwrapQoderSSELine("data: "+string(envelope), "qoder/auto")
			if done || data != body {
				t.Fatalf("completion was changed: done=%t data=%s", done, data)
			}
		})
	}
}

func TestQoderSSEBillingErrorsAreTerminal(t *testing.T) {
	for _, body := range []string{
		`{"code":"110","message":"quota exhausted"}`,
		`{"code": 112, "message":"quota exhausted"}`,
		`{"code": "10605", "message":"quota exhausted"}`,
		`{"error":{"message":"insufficient balance"}}`,
		`{"message":"account in arrears"}`,
		`{"pricingUrl":"https://example.com/pricing"}`,
	} {
		t.Run(body, func(t *testing.T) {
			envelope, err := json.Marshal(map[string]any{"statusCodeValue": 200, "body": body})
			if err != nil {
				t.Fatal(err)
			}
			data, done := UnwrapQoderSSELine("data: "+string(envelope), "qoder/auto")
			var result struct {
				Error struct {
					Code int `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(data), &result); err != nil || !done || result.Error.Code != 403 {
				t.Fatalf("expected terminal billing error: done=%t data=%s err=%v", done, data, err)
			}
		})
	}
}

func TestQoderSSEErrorIsProtocolErrorNotCompletion(t *testing.T) {
	for _, line := range []string{
		`data: {"statusCodeValue":429,"body":"rate limited"}`,
		`data: {"statusCodeValue":200,"body":"{\"error\":{\"message\":\"failed\"}}"}`,
		`data: {broken}`,
		`data: {"statusCodeValue":200,"body":"broken"}`,
	} {
		t.Run(line, func(t *testing.T) {
			data, done := UnwrapQoderSSELine(line, "qoder/auto")
			var result map[string]json.RawMessage
			if err := json.Unmarshal([]byte(data), &result); err != nil || !done || len(result["error"]) == 0 || result["choices"] != nil {
				t.Errorf("expected terminal protocol error, got done=%t data=%q err=%v", done, data, err)
			}
		})
	}
	for _, line := range []string{"data: [DONE]", `data: {"statusCodeValue":200,"body":"[DONE]"}`} {
		data, done := UnwrapQoderSSELine(line, "qoder/auto")
		if !done || data != "" {
			t.Errorf("normal DONE must not carry error data: done=%t data=%q", done, data)
		}
	}
}
