package handler

import (
	"sync/atomic"
	"time"
)

// requestDetailSeq disambiguates requestDetails rows that land in the same clock
// tick. Windows resolves time.Now() to ~15.6ms, so two media calls (a failed
// account and the retry) can otherwise mint the same "<nano>-<model>" id and the
// INSERT ... ON CONFLICT(id) DO UPDATE silently collapses them into one audit row.
var requestDetailSeq atomic.Int64

// nextRequestDetailStamp returns a strictly increasing value usable as the
// leading component of a requestDetails primary key.
func nextRequestDetailStamp() int64 {
	for {
		last := requestDetailSeq.Load()
		now := time.Now().UnixNano()
		next := now
		if next <= last {
			next = last + 1
		}
		if requestDetailSeq.CompareAndSwap(last, next) {
			return next
		}
	}
}
