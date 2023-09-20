// Package spsc implements a single-producer, single-consumer queue.
package spsc

// Implementation overview: The queue wraps a buffered channel; under normal
// operation the sender and receiver are just using this channel in the usual
// way, but if the channel fills up, instead of blocking, the sender will close
// the main channel, use a secondary "next" channel to send the receiver the
// next channel to read from (as part of a node, which includes the next "next"
// as well). next has buffer size 1 so this operation does not block either.
//
// If the receiver reaches the end of the items queue, it reads from "next"
// for more items.

import (
	"context"
	"io"
)

const itemsBuffer = 64

// A single-producer, single-consumer queue. Create one with New(),
// and send with Tx.Send(). Tx and Rx are each not safe for use by
// multiple goroutines, but two separate goroutines can use Tx and
// Rx respectively.
type Queue[T any] struct {
	Tx[T]
	Rx[T]
}

// The receive end of a Queue.
type Rx[T any] struct {
	head node[T]
}

// The send/transmit end of a Queue.
type Tx[T any] struct {
	// Pointer to the tail of the list. This will have a locked mu,
	// and zero values for other fields.
	tail node[T]
}

type node[T any] struct {
	items chan T
	next  chan node[T]
}

func newNode[T any]() node[T] {
	return node[T]{
		items: make(chan T, itemsBuffer),
		next:  make(chan node[T], 1),
	}
}

// Create a new, initially empty Queue.
func New[T any]() Queue[T] {
	n := newNode[T]()
	return Queue[T]{
		Rx: Rx[T]{head: n},
		Tx: Tx[T]{tail: n},
	}
}

// Send a message on the queue.
func (tx *Tx[T]) Send(v T) {
	for {
		select {
		case tx.tail.items <- v:
			return
		default:
			close(tx.tail.items)
			n := newNode[T]()
			tx.tail.next <- n
			tx.tail = n
		}
	}
}

// Close the queue. Calls to Recv on the other end will return io.EOF.
func (tx *Tx[T]) Close() error {
	close(tx.tail.items)
	close(tx.tail.next)
	return nil
}

// Receive a message from the queue. Blocks if the queue is empty.
// If the context ends before the receive happens, this returns
// ctx.Err(). If Close is called on the corresponding Tx, this
// returns io.EOF
func (rx *Rx[T]) Recv(ctx context.Context) (T, error) {
	for {
		select {
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		case v, ok := <-rx.head.items:
			if ok {
				return v, nil
			}
			rx.head, ok = <-rx.head.next
			if !ok {
				var zero T
				return zero, io.EOF
			}
		}
	}
}

// Try to receive a message from the queue. If successful, ok will be true.
// If the queue is empty, this will return immediately with ok = false.
func (rx *Rx[T]) TryRecv() (v T, ok bool) {
	for {
		select {
		case v, ok = <-rx.head.items:
			if !ok {
				rx.head = <-rx.head.next
				continue
			}
			return v, true
		default:
			return v, false
		}
	}
}
