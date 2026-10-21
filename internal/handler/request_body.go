package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"
)

// maxPreallocBody caps how much a Content-Length header is trusted to size the
// read buffer. A declared length far above the real payload would otherwise let
// a client make the gateway allocate that much up front, so anything past this
// grows incrementally instead.
const maxPreallocBody = 8 << 20

// readRequestBody reads the whole request body, pre-sizing the buffer from a
// plausible Content-Length. A large JSON body otherwise reallocates and copies
// on every growth step, which on a 1.6MB agent conversation is several copies
// of the same bytes before parsing even starts.
//
// The length is a hint only: chunked requests have none, and a wrong value must
// not truncate or over-read, so the body is still read to EOF.
func readRequestBody(r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, nil
	}
	if n, err := strconv.ParseInt(r.Header.Get("Content-Length"), 10, 64); err == nil && n > 0 && n <= maxPreallocBody {
		buf := make([]byte, 0, n)
		for {
			if len(buf) == cap(buf) {
				// The declared length was short; let append grow from here.
				rest, readErr := io.ReadAll(r.Body)
				buf = append(buf, rest...)
				if readErr != nil {
					return buf, readErr
				}
				return buf, nil
			}
			read, readErr := r.Body.Read(buf[len(buf):cap(buf)])
			buf = buf[:len(buf)+read]
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					return buf, nil
				}
				return buf, readErr
			}
		}
	}
	return io.ReadAll(r.Body)
}
