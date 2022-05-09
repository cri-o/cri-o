package flowcontrol

import (
	"context"
	"sync"

	"capnproto.org/go/capnp/v3/internal/chanmutex"
)

// Returns a FlowLimiter that enforces a fixed limit on the total size of
// outstanding messages.
func NewFixedLimiter(size uint64) FlowLimiter {
	return &fixedLimiter{
		total: size,
		avail: size,
	}
}

type fixedLimiter struct {
	mu           sync.Mutex
	total, avail uint64

	pending requestQueue
}

func (fl *fixedLimiter) StartMessage(ctx context.Context, size uint64) (gotResponse func(), err error) {
	gotResponse = fl.makeCallback(size)
	fl.mu.Lock()
	ready := fl.pending.put(size)
	fl.pumpQueue()
	fl.mu.Unlock()
	select {
	case <-ready:
		return gotResponse, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Unblock as many requests as the limit will allow.
//
// The caller must be holding fl.mu.
func (fl *fixedLimiter) pumpQueue() {
	next := fl.pending.peek()
	for next != nil && next.size <= fl.avail {
		fl.avail -= next.size
		fl.pending.next() // remove it from the queue
		next.ready.Unlock()
		next = fl.pending.peek()
	}
}

// Return a function which, when called, will return `size` bytes to the
// available pool.
func (fl *fixedLimiter) makeCallback(size uint64) func() {
	return func() {
		fl.mu.Lock()
		defer fl.mu.Unlock()
		fl.avail += size
		fl.pumpQueue()
	}
}

// A queue of requests to send.
type requestQueue struct {
	head, tail *request
}

// Node in the linked list that makes up requestQueue.
type request struct {
	// Unlock this when ready:
	ready chanmutex.Mutex

	size uint64
	next *request
}

// Look at the first request in the queue, without dequeueing it.
func (q *requestQueue) peek() *request {
	return q.head
}

// Drop the first request from the queue.
func (q *requestQueue) next() {
	if q.head != nil {
		q.head = q.head.next
	}
	if q.head == nil {
		q.tail = nil
	}
}

// Enqueue a request to send `size` bytes. Returns a locked mutex
// which will be unlocked when it is appropriate to send.
func (q *requestQueue) put(size uint64) chanmutex.Mutex {
	req := &request{
		ready: chanmutex.NewLocked(),
		size:  size,
		next:  nil,
	}
	if q.tail == nil {
		q.tail = req
		q.head = req
	} else {
		q.tail.next = req
		q.tail = req
	}
	return req.ready
}
