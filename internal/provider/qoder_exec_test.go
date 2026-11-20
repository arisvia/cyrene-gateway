package provider

import (
	"encoding/json"
	"testing"
)

func TestQoderSSEBillingDetectionPreservesCompletion(t *testing.T) {
	for _, body := range []string{
		`{"choices":[{"delta":{"content":"insufficient evidence of arrears"}}]}`,
		`{"choices":[{"delta":{"reasoning_content":"check pricingUrl first"}}]}`,
		`{"choices":[{"delta":{"content":"func checkStatus() { if isQueued { return } }"}}]}`,
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

func TestQoderSSEQueueingFramesAreSkipped(t *testing.T) {
	// Test various shapes of the upstream isQueued / 10605 queueing handshake frames
	cases := []struct {
		body      string
		wantRetry int
	}{
		{`{"code":"403","message":"{\"code\":\"10605\",\"message\":\"{\\\"isQueued\\\":true,\\\"modelKey\\\":\\\"dfmodel\\\",\\\"queueCount\\\":0,\\\"retryAfterSeconds\\\":7}\"}"}`, 7},
		{`{"isQueued":true,"modelKey":"dfmodel","queueCount":0}`, DefaultQoderQueueRetrySeconds},
		{`{"code":"10605","isQueued":true,"message":"queued"}`, DefaultQoderQueueRetrySeconds},
	}

	for _, tc := range cases {
		t.Run(tc.body, func(t *testing.T) {
			for _, status := range []int{200, 403} {
				envelope, err := json.Marshal(map[string]any{"statusCodeValue": status, "body": tc.body})
				if err != nil {
					t.Fatal(err)
				}
				line := "data: " + string(envelope)

				// UnwrapQoderSSELine must swallow the control frame.
				if data, done := UnwrapQoderSSELine(line, "qoder/auto"); done || data != "" {
					t.Fatalf("queueing frame should return done=false and data=\"\": got done=%t data=%s (status=%d)", done, data, status)
				}

				// The retry hint drives the transparent polling backoff.
				sig := ExtractQoderQueueSignalFromLine(line)
				if !sig.Queued {
					t.Fatalf("expected queue detection for status=%d", status)
				}
				if sig.RetryAfterSec != tc.wantRetry {
					t.Errorf("retryAfterSeconds = %d, want %d", sig.RetryAfterSec, tc.wantRetry)
				}
				if sig.PlanRefusal() {
					t.Errorf("envelope without serviceAvailable=false must not read as a plan refusal: %s", tc.body)
				}
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

// qoderNestedEnvelope reproduces the upstream triple nesting measured on
// production 2026-11-20: the queue object sits inside a stringified
// {"code":"10605",…,"subCode":…}, itself inside {"code":"403","message":…}.
func qoderNestedEnvelope(t *testing.T, inner map[string]any) string {
	t.Helper()
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		t.Fatal(err)
	}
	mid, err := json.Marshal(map[string]any{"code": "10605", "message": string(innerJSON), "subCode": "RESOURCE_LIMIT"})
	if err != nil {
		t.Fatal(err)
	}
	outer, err := json.Marshal(map[string]any{"code": "403", "message": string(mid)})
	if err != nil {
		t.Fatal(err)
	}
	return string(outer)
}

func TestQoderQueueSignalSeparatesPlanRefusalFromCongestion(t *testing.T) {
	refusal := qoderNestedEnvelope(t, map[string]any{
		"isQueued": true, "modelKey": "qmodel_latest", "queueCount": 0, "queueType": "p2",
		"retryAfterSeconds": 26, "serviceAvailable": false, "subCode": "RESOURCE_LIMIT", "waitTime": 26,
	})
	congestion := qoderNestedEnvelope(t, map[string]any{
		"isQueued": true, "modelKey": "auto", "queueCount": 1, "queueType": "p4",
		"retryAfterSeconds": 60, "serviceAvailable": true, "subCode": "RESOURCE_LIMIT", "waitTime": 60,
	})

	sig := ExtractQoderQueueSignal(refusal)
	if !sig.Queued || !sig.PlanRefusal() {
		t.Fatalf("serviceAvailable=false must decode as queued plan refusal: %+v", sig)
	}
	if sig.RetryAfterSec != 26 || sig.SubCode != "RESOURCE_LIMIT" {
		t.Errorf("decoded fields = %d/%q, want 26/RESOURCE_LIMIT", sig.RetryAfterSec, sig.SubCode)
	}

	sig = ExtractQoderQueueSignal(congestion)
	if !sig.Queued {
		t.Fatal("congestion frame must stay queued")
	}
	if sig.PlanRefusal() {
		t.Fatal("a moving lane (serviceAvailable=true) keeps wait semantics even under subCode RESOURCE_LIMIT")
	}

	envelope, err := json.Marshal(map[string]any{"statusCodeValue": 403, "body": refusal})
	if err != nil {
		t.Fatal(err)
	}
	if sig := ExtractQoderQueueSignalFromLine("data: " + string(envelope)); !sig.PlanRefusal() {
		t.Fatalf("SSE line form must decode identically: %+v", sig)
	}
}
