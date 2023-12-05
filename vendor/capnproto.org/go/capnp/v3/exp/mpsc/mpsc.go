// Package mpsc implements a multiple-producer, single-consumer queue.
package mpsc

// N.B. this is a trivial wrapper around the spsc package, which just adds
// a lock to the Tx end to make it multiple-producer.

import (
	"context"
	"sync"

	"capnproto.org/go/capnp/v3/exp/spsc"
)

// A multiple-producer, single-consumer queue. Create one with New(),
// and send from many gorotuines with Tx.Send(). Only one gorotuine may
// call Rx.Recv().
type Queue[T any] struct {
	Tx[T]
	Rx[T]
}

// The receive end of a Queue.
type Rx[T any] struct {
	rx spsc.Rx[T]
}

// The send/transmit end of a Queue.
type Tx[T any] struct {
	mu sync.Mutex
	tx spsc.Tx[T]
}

// Create a new, initially empty Queue.
func New[T any]() *Queue[T] {
	q := spsc.New[T]()
	return &Queue[T]{
		Tx: Tx[T]{tx: q.Tx},
		Rx: Rx[T]{rx: q.Rx},
	}
}

// Send a message on the queue.
func (tx *Tx[T]) Send(v T) {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	tx.tx.Send(v)
}

// Close the queue. Calls to Recv on the other end will return io.EOF.
func (tx *Tx[T]) Close() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	return tx.tx.Close()
}

// Receive a message from the queue. Blocks if the queue is empty.
// If the context ends before the receive happens, this returns
// ctx.Err(). If Close is called on the corresponding Tx, this
// returns io.EOF
func (rx *Rx[T]) Recv(ctx context.Context) (T, error) {
	return rx.rx.Recv(ctx)
}

// Try to receive a message from the queue. If successful, ok will be true.
// If the queue is empty, this will return immediately with ok = false.
func (rx *Rx[T]) TryRecv() (v T, ok bool) {
	return rx.rx.TryRecv()
}
