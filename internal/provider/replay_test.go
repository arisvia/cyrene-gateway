package provider

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"syscall"
	"testing"
)

// timeoutError stands in for the unexported http.httpError that
// Transport.ResponseHeaderTimeout produces: a net.Error whose Timeout reports
// true and which carries no *net.OpError wrapper.
type timeoutError struct{}

func (timeoutError) Error() string   { return "net/http: timeout awaiting response headers" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return false }

func upstreamDoError(inner error) error {
	return &url.Error{Op: "Post", URL: "https://upstream.example/v1/chat/completions", Err: inner}
}

// A keep-alive the upstream edge dropped silently must be retried on a fresh
// socket; anything where upstream may already hold the request must not be,
// because replaying it bills a second completion.
func TestIsReplayableTransportError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"reset on read after idle", upstreamDoError(&net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}), true},
		{"reset on write", upstreamDoError(&net.OpError{Op: "write", Net: "tcp", Err: syscall.ECONNRESET}), true},
		{"broken pipe on write", upstreamDoError(&net.OpError{Op: "write", Net: "tcp", Err: syscall.EPIPE}), true},
		{"eof on reused socket", upstreamDoError(io.EOF), true},
		{"unexpected eof", upstreamDoError(io.ErrUnexpectedEOF), true},
		{"transport closed", upstreamDoError(net.ErrClosed), true},
		{"write to retired socket", upstreamDoError(&net.OpError{Op: "write", Net: "tcp", Err: net.ErrClosed}), true},
		{"bare eof string", upstreamDoError(errors.New("read tcp 10.0.0.5:5210->142.250.0.1:443: read: EOF")), true},
		{"reset during dial", upstreamDoError(&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNRESET}), true},
		// Not replayable: upstream may already be generating, or an instant retry
		// would fail on the same broken route and only delay failover.
		{"response header timeout", upstreamDoError(timeoutError{}), false},
		{"dial timeout", upstreamDoError(&net.OpError{Op: "dial", Net: "tcp", Err: timeoutError{}}), false},
		{"connection refused", upstreamDoError(&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}), false},
		{"dns failure", upstreamDoError(&net.DNSError{Err: "no such host", Name: "upstream.example"}), false},
		{"tls alert", upstreamDoError(errors.New("remote error: tls: bad certificate")), false},
		{"client cancel", upstreamDoError(context.Canceled), false},
		{"client deadline", upstreamDoError(context.DeadlineExceeded), false},
		{"unrelated error", errors.New("no active credentials"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsReplayableTransportError(tc.err); got != tc.want {
				t.Errorf("IsReplayableTransportError(%v) = %t, want %t", tc.err, got, tc.want)
			}
		})
	}
}
