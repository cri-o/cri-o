// Package maybe provides support for working with optional values.
package maybe

// A Maybe[V] represents an optional value of type V. The zero value for Maybe[V]
// is considered a "missing" value.
type Maybe[V any] struct {
	value V
	ok    bool
}

// Create a new, non-empty Maybe with value 'value'.
func New[V any](value V) Maybe[V] {
	return Maybe[V]{
		value: value,
		ok:    true,
	}
}

// Get the underlying value, if any. ok is true iff the value was present.
func (t Maybe[V]) Get() (value V, ok bool) {
	return t.value, t.ok
}
