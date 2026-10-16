package provider

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestStallTimeoutReader_ActiveStream(t *testing.T) {
	pr, pw := io.Pipe()
	sr := NewStallTimeoutReader(pr, 200*time.Millisecond)
	defer sr.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 4 {
			time.Sleep(50 * time.Millisecond)
			pw.Write([]byte("chunk\n"))
		}
		pw.Close()
	}()

	buf := make([]byte, 64)
	total := 0
	for {
		n, err := sr.Read(buf)
		total += n
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("unexpected read error: %v", err)
		}
	}
	<-done
	if total != 24 { // 4 * len("chunk\n")
		t.Fatalf("expected 24 bytes read, got %d", total)
	}
}

func TestStallTimeoutReader_StallTriggered(t *testing.T) {
	pr, pw := io.Pipe()
	sr := NewStallTimeoutReader(pr, 80*time.Millisecond)
	defer sr.Close()
	defer pw.Close()

	// Initial data
	go func() {
		pw.Write([]byte("first\n"))
		// Intentionally pause longer than 80ms
		time.Sleep(200 * time.Millisecond)
		pw.Write([]byte("second\n"))
	}()

	buf := make([]byte, 64)
	n, err := sr.Read(buf)
	if err != nil || string(buf[:n]) != "first\n" {
		t.Fatalf("first read failed: n=%d err=%v", n, err)
	}

	// Second read should detect stall
	_, err2 := sr.Read(buf)
	if err2 == nil {
		t.Fatal("expected stall error on second read, got nil")
	}
	if !errors.Is(err2, ErrStreamStalled) && !strings.Contains(err2.Error(), "stalled") && !strings.Contains(err2.Error(), "closed") {
		t.Fatalf("expected stall error, got: %v", err2)
	}
}
