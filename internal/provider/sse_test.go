package provider

import (
	"context"
	"strings"
	"testing"
)

func TestSSEReaderMultiLineAndComments(t *testing.T) {
	input := `: this is a comment
event: update
id: 123
data: first line
data: second line

data: single line payload

`
	reader := NewSSEReader(strings.NewReader(input))
	ctx := context.Background()

	// First event
	ev1, err := reader.ReadEvent(ctx)
	if err != nil {
		t.Fatalf("failed to read ev1: %v", err)
	}
	if ev1.Event != "update" || ev1.ID != "123" {
		t.Errorf("unexpected ev1 headers: %+v", ev1)
	}
	if string(ev1.Data) != "first line\nsecond line" {
		t.Errorf("expected multiline data, got %q", string(ev1.Data))
	}

	// Second event
	ev2, err := reader.ReadEvent(ctx)
	if err != nil {
		t.Fatalf("failed to read ev2: %v", err)
	}
	if string(ev2.Data) != "single line payload" {
		t.Errorf("expected single line, got %q", string(ev2.Data))
	}
}
