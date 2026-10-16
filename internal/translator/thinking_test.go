package translator

import (
	"strings"
	"testing"
)

func TestThinkTagState_Streaming(t *testing.T) {
	ts := &ThinkTagState{}

	// 1. Tag split across chunk boundaries: "<thi" then "nk>First thought"
	c1 := ts.ProcessFeed("<thi")
	if len(c1) != 0 {
		t.Fatalf("expected prefix '<thi' to be buffered, got: %+v", c1)
	}

	c2 := ts.ProcessFeed("nk>Step 1 analysis")
	if len(c2) != 1 || c2[0].Type != DeltaThinking || c2[0].Delta != "Step 1 analysis" {
		t.Fatalf("expected thinking delta, got: %+v", c2)
	}

	// 2. Normal thinking chunk
	c3 := ts.ProcessFeed(" continue thought")
	if len(c3) != 1 || c3[0].Type != DeltaThinking || c3[0].Delta != " continue thought" {
		t.Fatalf("expected thinking delta, got: %+v", c3)
	}

	// 3. Closing tag split across boundaries: "</thi" then "nk>Final answer"
	c4 := ts.ProcessFeed("</thi")
	if len(c4) != 0 {
		t.Fatalf("expected closing prefix to be buffered, got: %+v", c4)
	}

	c5 := ts.ProcessFeed("nk>Final answer")
	if len(c5) != 1 || c5[0].Type != DeltaText || c5[0].Delta != "Final answer" {
		t.Fatalf("expected text delta, got: %+v", c5)
	}

	// 4. Normal text chunk
	c6 := ts.ProcessFeed(" is ready.")
	if len(c6) != 1 || c6[0].Type != DeltaText || c6[0].Delta != " is ready." {
		t.Fatalf("expected text delta, got: %+v", c6)
	}

	flushed := ts.Flush()
	if len(flushed) != 0 {
		t.Fatalf("expected clean flush, got: %+v", flushed)
	}
}

func TestThinkTagState_FastPath(t *testing.T) {
	ts := &ThinkTagState{}
	// Chunks without '<' must take fast path
	deltas := ts.ProcessFeed("plain text with no tags at all")
	if len(deltas) != 1 || deltas[0].Type != DeltaText || deltas[0].Delta != "plain text with no tags at all" {
		t.Fatalf("fast path failed: %+v", deltas)
	}
}

func TestStripThinkTags(t *testing.T) {
	raw := "<think>\nThinking through the solution.\nStep 1: Check inputs.\n</think>\n\nHere is the actual result."
	thinking, clean := StripThinkTags(raw)
	if !strings.Contains(thinking, "Step 1: Check inputs.") {
		t.Errorf("thinking not extracted: %q", thinking)
	}
	if strings.Contains(clean, "<think>") || strings.Contains(clean, "</think>") {
		t.Errorf("think tags leaked into clean text: %q", clean)
	}
	if clean != "Here is the actual result." {
		t.Errorf("unexpected clean text: %q", clean)
	}
}
