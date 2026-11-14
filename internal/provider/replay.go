package provider

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"syscall"
)

// IsReplayableTransportError reports whether an upstream http.Client.Do failure
// can be replayed on a fresh connection without risking a duplicated completion.
//
// Cloudflare, Google's frontends and the NIM gateways in front of most coding
// APIs drop idle keep-alive sockets silently — no close_notify, no RST — so the
// first write onto a poisoned socket fails with ECONNRESET/EPIPE/EOF before the
// request ever reached the model, and the retry costs nothing.
//
// The complement is deliberately excluded: once the request has been delivered,
// replaying it can double-bill a completion or re-run tool side effects. So a
// timeout with no response headers (the upstream had the request and may still be
// generating), a caller-side cancel, and route-level faults that a second instant
// attempt would fail identically on (DNS, refused, TLS) all return false and let
// cross-account failover take the request instead.
//
// Same classification 9router shipped for their Antigravity edge (963787f73f).
func IsReplayableTransportError(err error) bool {
	if err == nil {
		return false
	}
	// The caller hung up, or the client deadline expired after the request was
	// sent: neither is a stale socket, and replaying buys nothing.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Any timeout means the client waited on a live socket path (connect stall or
	// ResponseHeaderTimeout after the request was written). Replaying instantly
	// gets the same stall, and the latter may already be generating upstream.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return false
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, net.ErrClosed) {
			// The runtime retired the socket under us: nothing was sent.
			return true
		}
		switch opErr.Op {
		case "dial":
			// Connect-phase faults never delivered a byte. A refused port is the
			// exception: the route is down and an instant retry hits the same wall.
			return !errors.Is(opErr.Err, syscall.ECONNREFUSED)
		case "read", "write":
			return errors.Is(opErr.Err, syscall.ECONNRESET) || errors.Is(opErr.Err, syscall.EPIPE)
		}
		return false
	}

	// A dead reused connection also surfaces unwrapped: net.ErrClosed for a socket
	// the runtime already retired, or a bare EOF from the transport's read loop.
	if errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}
	msg := err.Error()
	return strings.HasSuffix(msg, ": EOF") ||
		strings.Contains(msg, "use of closed network connection")
}
