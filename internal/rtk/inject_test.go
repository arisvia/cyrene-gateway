package rtk

import "testing"

func TestInjectSystemPromptUsesThePartTypeOfEachShape(t *testing.T) {
	tests := []struct {
		name       string
		body       map[string]any
		key        string
		wantType   string
		wantInsert string
	}{
		{
			name: "chat completions messages array",
			body: map[string]any{"messages": []any{
				map[string]any{"role": "system", "content": []any{map[string]any{"type": "text", "text": "base"}}},
			}},
			key: "messages", wantType: "text",
		},
		{
			name: "responses input array",
			body: map[string]any{"input": []any{
				map[string]any{"role": "developer", "content": []any{map[string]any{"type": "input_text", "text": "base"}}},
			}},
			key: "input", wantType: "input_text", wantInsert: "developer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			InjectSystemPrompt(tt.body, "openai", "injected")
			arr, ok := tt.body[tt.key].([]any)
			if !ok || len(arr) == 0 {
				t.Fatalf("expected %s array to survive, got %T", tt.key, tt.body[tt.key])
			}
			// The system/developer message is the first entry in both fixtures.
			msg := arr[0].(map[string]any)
			parts := msg["content"].([]any)
			last := parts[len(parts)-1].(map[string]any)
			if last["text"] != "injected" {
				t.Fatalf("expected the prompt to be appended, got %v", last)
			}
			if last["type"] != tt.wantType {
				t.Errorf("part type = %v, want %q", last["type"], tt.wantType)
			}
		})
	}
}

func TestInjectSystemPromptEmptyStringContent(t *testing.T) {
	body := map[string]any{"messages": []any{
		map[string]any{"role": "system", "content": ""},
	}}
	InjectSystemPrompt(body, "openai", "injected")
	if got := body["messages"].([]any)[0].(map[string]any)["content"]; got != "injected" {
		t.Errorf("an empty system message must receive the prompt alone, got %q", got)
	}
}
