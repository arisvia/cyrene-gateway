package provider

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// SSEEvent represents a single standard SSE event.
type SSEEvent struct {
	ID    string
	Event string
	Data  []byte
	Retry string
}

// SSEReader parses server-sent events according to standard SSE spec.
type SSEReader struct {
	reader *bufio.Reader
}

func NewSSEReader(r io.Reader) *SSEReader {
	return &SSEReader{reader: bufio.NewReaderSize(r, 64*1024)}
}

var (
	ErrScannerBufferExceeded = errors.New("sse event line exceeds buffer limit")
)

// ReadEvent reads the next complete SSE event block.
func (r *SSEReader) ReadEvent(ctx context.Context) (*SSEEvent, error) {
	var event SSEEvent
	var dataLines [][]byte

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		line, err := r.reader.ReadBytes('\n')
		if err != nil && len(line) == 0 {
			if len(dataLines) > 0 || event.Event != "" || event.ID != "" {
				event.Data = bytes.Join(dataLines, []byte("\n"))
				return &event, nil
			}
			return nil, err
		}

		// Strip trailing \r and \n
		line = bytes.TrimRight(line, "\r\n")

		// Empty line marks end of an event block
		if len(line) == 0 {
			if len(dataLines) > 0 || event.Event != "" || event.ID != "" {
				event.Data = bytes.Join(dataLines, []byte("\n"))
				return &event, nil
			}
			if err != nil {
				return nil, err
			}
			continue
		}

		// Comment line
		if line[0] == ':' {
			if err != nil {
				return nil, err
			}
			continue
		}

		colon := bytes.IndexByte(line, ':')
		var field, value []byte
		if colon == -1 {
			field = line
			value = []byte{}
		} else {
			field = line[:colon]
			value = line[colon+1:]
			if len(value) > 0 && value[0] == ' ' {
				value = value[1:]
			}
		}

		switch string(field) {
		case "data":
			dataLines = append(dataLines, value)
		case "event":
			event.Event = string(value)
		case "id":
			event.ID = string(value)
		case "retry":
			event.Retry = string(value)
		}

		if err != nil {
			if len(dataLines) > 0 || event.Event != "" || event.ID != "" {
				event.Data = bytes.Join(dataLines, []byte("\n"))
				return &event, nil
			}
			return nil, err
		}
	}
}

// FormatSSEEvent formats an event into standard SSE text.
func FormatSSEEvent(e SSEEvent) []byte {
	var buf bytes.Buffer
	if e.Event != "" {
		buf.WriteString(fmt.Sprintf("event: %s\n", e.Event))
	}
	if e.ID != "" {
		buf.WriteString(fmt.Sprintf("id: %s\n", e.ID))
	}
	if e.Retry != "" {
		buf.WriteString(fmt.Sprintf("retry: %s\n", e.Retry))
	}

	lines := strings.Split(string(e.Data), "\n")
	for _, l := range lines {
		buf.WriteString(fmt.Sprintf("data: %s\n", l))
	}
	buf.WriteString("\n")
	return buf.Bytes()
}
