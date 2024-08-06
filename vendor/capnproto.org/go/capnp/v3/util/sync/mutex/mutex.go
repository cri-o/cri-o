// Package mutex provides mutexes that wrap the thing they protect.
// This results in clearer code than using sync.Mutex directly.
package mutex

import "sync"

// A Mutex[T] wraps a T, protecting it with a mutex. It must not
// be moved after first use. The zero value is an unlocked mutex
// containing the zero value of T.
type Mutex[T any] struct {
	mu  sync.Mutex
	val T
}

// New returns a new mutex containing the value val.
func New[T any](val T) Mutex[T] {
	return Mutex[T]{val: val}
}

// With invokes the callback with exclusive access to the value of the mutex.
func (m *Mutex[T]) With(f func(*T)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f(&m.val)
}

// Lock acquires the mutex and returns a reference to the value. Call Unlock()
// on the return value to release the lock.
//
// Where possible, you should prefer using Mutex.With and similar functions. If
// those are insufficient to handle your use case consider whether your locking
// scheme is too complicated before going ahead and using this.
func (m *Mutex[T]) Lock() *Locked[T] {
	m.mu.Lock()
	return &Locked[T]{mu: m}
}

// With1(m, ...) is like m.With(...), except that the callback returns
// a value.
func With1[T, A any](m *Mutex[T], f func(*T) A) A {
	var ret A
	m.With(func(t *T) {
		ret = f(t)
	})
	return ret
}

// With2 is like With1, but the callback returns two values instead of one.
func With2[T, A, B any](m *Mutex[T], f func(*T) (A, B)) (A, B) {
	var (
		a A
		b B
	)
	m.With(func(t *T) {
		a, b = f(t)
	})
	return a, b
}

// With3 is like With1, but the callback returns three values instead of one.
func With3[T, A, B, C any](m *Mutex[T], f func(*T) (A, B, C)) (A, B, C) {
	var (
		a A
		b B
		c C
	)
	m.With(func(t *T) {
		a, b, c = f(t)
	})
	return a, b, c
}

// With4 is like With1, but the callback returns four values instead of one.
func With4[T, A, B, C, D any](m *Mutex[T], f func(*T) (A, B, C, D)) (A, B, C, D) {
	var (
		a A
		b B
		c C
		d D
	)
	m.With(func(t *T) {
		a, b, c, d = f(t)
	})
	return a, b, c, d
}

// A Locked[T] is a reference to a value of type T which is guarded by a Mutex,
// which the caller has acquired.
type Locked[T any] struct {
	mu *Mutex[T]
}

// Value returns a reference to the protected value. It must not be used after
// calling Unlock.
//
// Value will panic if called after Unlock()
//
// We recommend against assigning the result of Value() to a variable; while
// repeatedly having to write l.Value() is mildly annoying, it makes it
// much harder to accidentally use the value after unlocking.
func (l *Locked[T]) Value() *T {
	return &l.mu.val
}

// Unlock releases the mutex. Any references to the value obtained via Value
// must not be used after this is called.
func (l *Locked[T]) Unlock() {
	l.mu.mu.Unlock()
	l.mu = nil
}
