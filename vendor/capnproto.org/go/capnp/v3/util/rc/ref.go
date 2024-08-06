// Package rc provides reference-counted cells.
package rc

import "sync/atomic"

// A Ref is a reference to a refcounted cell containing a T.
// It must not be moved after it is first used (but it is ok
// to move the return values of NewRef/AddRef *before* using
// them). The zero value is an already-released reference to
// some non-existant cell; it is not useful.
type Ref[T any] struct {
	// Pointer to the actual data. When we release our
	// reference, we set this to nil so we can't access
	// it.
	cell *cell[T]
}

// Container for the actual data; Ref just points to this.
type cell[T any] struct {
	value    T      // The actual value that is stored.
	refcount int32  // The refernce count.
	release  func() // Function to call when refcount hits zero.
}

// NewRef returns a Ref pointing to value. When all references
// are released, the function release will be called.
func NewRef[T any](value T, release func()) *Ref[T] {
	return &Ref[T]{
		cell: &cell[T]{
			value:    value,
			refcount: 1,
			release:  release,
		},
	}
}

// NewRefInPlace returns a ref pointing at a value. It constructs the
// value by passing a pointer to the zero value to the mk callback,
// which should return the function to run on release.
//
// This is useful when the value cannot be moved after construction,
// but otherwise you will likely find NewRef more convinenet.
func NewRefInPlace[T any](mk func(v *T) (release func())) *Ref[T] {
	ret := &Ref[T]{
		cell: &cell[T]{
			refcount: 1,
		},
	}
	ret.cell.release = mk(&ret.cell.value)
	return ret
}

// AddRef returns a new reference to the same underlying data as
// the receiver. The references are not interchangable: to
// release the underlying data you must call Release on each
// Ref separately, and you cannot access the value through
// a released Ref even if you know there are other live references
// to it.
//
// Panics if this reference has already been released.
func (r *Ref[T]) AddRef() *Ref[T] {
	if r.cell == nil {
		panic("called AddRef() on already-released Ref.")
	}
	atomic.AddInt32(&r.cell.refcount, 1)
	return &Ref[T]{cell: r.cell}
}

// Release this reference to the value. If this is the last reference,
// this calls the release function that was passed to NewRef.
//
// Release is idempotent: calling it twice on the same reference
// has no effect. This is handy as it allows you to defer a call
// to Release and then still have the option of a releasing a
// reference early.
func (r *Ref[T]) Release() {
	if r == nil || r.cell == nil {
		// Already released.
		return
	}
	val := atomic.AddInt32(&r.cell.refcount, -1)
	if val == 0 {
		r.cell.release()
	}
	r.cell = nil
}

// Return a pointer to the value. Panics if the reference has already
// been released.
func (r *Ref[T]) Value() *T {
	return &r.cell.value
}

// Steal steals the receiver, releasing it and returning a different
// reference to the same cell. The refcount is unchanged, but this
// is useful to enforce ownership invariants.
func (r *Ref[T]) Steal() *Ref[T] {
	ret := &Ref[T]{cell: r.cell}
	r.cell = nil
	return ret
}

// Return true iff this is a valid ref, i.e. Value() will return without
// panicking. You may call this on a nil reference.
func (r *Ref[T]) IsValid() bool {
	return r != nil && r.cell != nil
}

// A WeakRef is a reference that does not keep the value alive.
// It can be used to obtain a (strong) Ref to the value if it
// has not yet been released.
type WeakRef[T any] cell[T]

// Weak returns a weak reference to the value.
func (r *Ref[T]) Weak() *WeakRef[T] {
	if r.cell == nil {
		panic("Called Weak() on already-released Ref")
	}
	return (*WeakRef[T])(r.cell)
}

// AddRef returns a (strong) Ref to the value, with ok indicating
// whether the operation was successfull -- if ok is false, this
// indicates that  underlying cell had already been released, and
// the returned Ref will be nil.
func (r *WeakRef[T]) AddRef() (_ *Ref[T], ok bool) {
	for {
		old := atomic.LoadInt32(&r.refcount)
		if old == 0 {
			return nil, false
		}
		if atomic.CompareAndSwapInt32(&r.refcount, old, old+1) {
			return &Ref[T]{cell: (*cell[T])(r)}, true
		}
	}
}
