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

func TestFormatSSEEventBytes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		event SSEEvent
		want  string
	}{
		{"nil", SSEEvent{}, "data: \n\n"},
		{"empty", SSEEvent{Data: []byte{}}, "data: \n\n"},
		{"single", SSEEvent{Data: []byte("hello")}, "data: hello\n\n"},
		{"multiline", SSEEvent{Data: []byte("first\nsecond")}, "data: first\ndata: second\n\n"},
		{"trailing newline", SSEEvent{Data: []byte("first\n")}, "data: first\ndata: \n\n"},
		{"blank lines", SSEEvent{Data: []byte("\n\n")}, "data: \ndata: \ndata: \n\n"},
		{"all fields", SSEEvent{Event: "update", ID: "123", Retry: "1000", Data: []byte("a\nb")}, "event: update\nid: 123\nretry: 1000\ndata: a\ndata: b\n\n"},
		{"raw bytes", SSEEvent{Data: []byte{'%', '\r', '\n', 0, 0xff}}, "data: %\r\ndata: \x00\xff\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(FormatSSEEvent(tc.event)); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestFormatSSEEventAllocationBudget(t *testing.T) {
	event := SSEEvent{Event: "update", ID: "123", Retry: "1000", Data: []byte("a\nb\n")}
	if allocs := testing.AllocsPerRun(20, func() { FormatSSEEvent(event) }); allocs > 1 {
		t.Fatalf("%.0f allocations: formatting should allocate only its output buffer", allocs)
	}
}

func BenchmarkFormatSSEEvent(b *testing.B) {
	for _, tc := range []struct {
		name string
		data string
	}{
		{"single", `{"type":"content_block_delta","delta":{"text":"hello"}}`},
		{"multiline", strings.Repeat("streaming payload\n", 16)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			event := SSEEvent{Event: "update", ID: "123", Retry: "1000", Data: []byte(tc.data)}
			b.ReportAllocs()
			for b.Loop() {
				FormatSSEEvent(event)
			}
		})
	}
}
