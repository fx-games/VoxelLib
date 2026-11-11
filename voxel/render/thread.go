package render

import (
	"bytes"
	"fmt"
	"runtime"
	"strconv"
	"sync"
)

// renderG is the goroutine Init ran on: the render thread. GPU objects are
// created only there (the GL backend's context is current on that thread;
// creating from another one fails with "in use" and crashes later).
var renderG uint64

// goroutineID parses the current goroutine's id from its stack header.
func goroutineID() uint64 {
	var b [64]byte
	s := b[:runtime.Stack(b[:], false)]
	s = bytes.TrimPrefix(s, []byte("goroutine "))
	id, _ := strconv.ParseUint(string(s[:bytes.IndexByte(s, ' ')]), 10, 64)
	return id
}

// mustRenderThread panics when a GPU object is created off the render
// thread (after Init): queue the work for the render thread instead.
func mustRenderThread(what string) {
	if renderG != 0 && goroutineID() != renderG {
		panic(fmt.Sprintf("render: %s created off the render thread", what))
	}
}

// gpuQueue lets a renderer's API run on any goroutine (e.g. the game
// tick): it queues its GPU work in pending, under mu, and the renderer's
// draws (on the render thread) run it first (lockFlush).
type gpuQueue struct {
	mu      sync.Mutex
	pending []func()
}

// queue adds GPU work for the next draw.
func (q *gpuQueue) queue(f func()) {
	q.mu.Lock()
	q.pending = append(q.pending, f)
	q.mu.Unlock()
}

// lockFlush takes mu and runs the queued GPU work (render thread); the
// caller unlocks mu when its draw is done.
func (q *gpuQueue) lockFlush() {
	q.mu.Lock()
	for _, f := range q.pending {
		f()
	}
	q.pending = q.pending[:0]
}
