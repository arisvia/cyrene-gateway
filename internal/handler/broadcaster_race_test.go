package handler

import (
	"sync"
	"testing"
	"time"
)

func TestEventBroadcaster_ConcurrentRace(t *testing.T) {
	b := NewEventBroadcaster()
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Publisher constantly publishing events
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				b.Publish(RequestEvent{
					Type:      "request",
					Timestamp: time.Now().Format(time.RFC3339),
					Model:     "test-model",
				})
			}
		}
	}()

	// 10 concurrent subscribers constantly subscribing, reading or unsubscribing
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				ch, done := b.Subscribe()
				// Read a little bit or unsubscribe immediately
				select {
				case <-ch:
				case <-time.After(50 * time.Microsecond):
				}
				b.Unsubscribe(ch)
				<-done
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestLogRingBuffer_ConcurrentRace(t *testing.T) {
	b := NewLogRingBuffer(100)
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Publisher constantly adding logs
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				b.Add(LogRecord{
					Timestamp: time.Now().Format(time.RFC3339),
					Message:   "test log message",
					Level:     "INFO",
				})
			}
		}
	}()

	// 10 concurrent subscribers constantly subscribing, reading or unsubscribing
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				ch, done := b.Subscribe()
				select {
				case <-ch:
				case <-time.After(50 * time.Microsecond):
				}
				b.Unsubscribe(ch)
				<-done
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestLogRingBuffer_KeepsNewestLinesWhenSubscriberLags(t *testing.T) {
	b := NewLogRingBuffer(5)
	ch, done := b.Subscribe()
	defer b.Unsubscribe(ch)
	defer func() { <-done }()

	// Overflow the 128-slot subscriber channel so the drop path runs.
	for i := 0; i < 400; i++ {
		b.Add(LogRecord{Timestamp: time.Now().Format(time.RFC3339), Level: "INFO", Message: "line"})
	}

	// The newest line must survive: a lagging tail that keeps ancient lines and
	// discards current ones shows the wrong end of the stream.
	var last LogRecord
	for len(ch) > 0 {
		last = <-ch
	}
	if last.Message != "line" {
		t.Fatalf("drained without reaching the newest record: %+v", last)
	}

	// The ring must also stay at capacity with in-place eviction.
	if got := len(b.GetAll()); got != 5 {
		t.Fatalf("ring size %d, want 5", got)
	}
}
