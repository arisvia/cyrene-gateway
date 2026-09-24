package provider

import (
	"encoding/json"
	"testing"
)

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
