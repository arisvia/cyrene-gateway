package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// reader that reports a length different from what it actually yields, to prove
// the Content-Length hint can never truncate or over-read the body.
type lyingReader struct {
	data string
	pos  int
}

func (l *lyingReader) Read(p []byte) (int, error) {
	if l.pos >= len(l.data) {
		return 0, io.EOF
	}
	n := copy(p, l.data[l.pos:])
	l.pos += n
	return n, nil
}

func bodyRequest(declared string, body io.Reader) *http.Request {
	r := httptest.NewRequest("POST", "/v1/chat/completions", body)
	r.Header.Del("Content-Length")
	if declared != "" {
		r.Header.Set("Content-Length", declared)
	}
	return r
}

func TestReadRequestBody_MatchesReadAll(t *testing.T) {
	payload := strings.Repeat(`{"role":"user","content":"hi"},`, 500)

	cases := []struct {
		name     string
		declared string
	}{
		{"exact length", ""}, // set below
		{"no content-length (chunked)", ""},
		{"declared shorter than body", "16"},
		{"declared longer than body", "100000"},
		{"declared not a number", "banana"},
		{"declared negative", "-1"},
		{"declared above prealloc cap", "999999999"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			declared := tc.declared
			if tc.name == "exact length" {
				declared = ""
			}
			r := bodyRequest(declared, &lyingReader{data: payload})
			if tc.name == "exact length" {
				r.Header.Set("Content-Length", itoa(len(payload)))
			}

			got, err := readRequestBody(r)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != payload {
				t.Fatalf("body mismatch: got %d bytes, want %d", len(got), len(payload))
			}
		})
	}
}

func TestReadRequestBody_EmptyBody(t *testing.T) {
	r := bodyRequest("0", bytes.NewReader(nil))
	got, err := readRequestBody(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d bytes, want 0", len(got))
	}
}

func TestReadRequestBody_NilBody(t *testing.T) {
	got, err := readRequestBody(&http.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
