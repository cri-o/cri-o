package flowcontrol

import (
	"context"

	"capnproto.org/go/capnp/v3/internal/str"
	"golang.org/x/sync/semaphore"
)

// Returns a FlowLimiter that enforces a fixed limit on the total size of
// outstanding messages.
func NewFixedLimiter(size int64) FlowLimiter {
	return &fixedLimiter{
		size: size,
		sem:  semaphore.NewWeighted(size),
	}
}

type fixedLimiter struct {
	size int64
	sem  *semaphore.Weighted
}

func (fl *fixedLimiter) StartMessage(ctx context.Context, size uint64) (gotResponse func(), err error) {
	// HACK:  avoid dead-locking if the size of the message exceeds the maximum
	//        reservation on the semaphore. We can't return an error because it
	//        is currently ignored by the caller.
	if int64(size) > fl.size {
		panic("StartMessage(): message size " +
			str.Utod(size) +
			" is too large (max " + str.Itod(fl.size) + ")")
	}

	if err = fl.sem.Acquire(ctx, int64(size)); err == nil {
		gotResponse = func() { fl.sem.Release(int64(size)) }
	}

	return
}

func (fixedLimiter) Release() {}
