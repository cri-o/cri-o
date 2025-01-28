package memorystore

import (
	"sort"
)

// History is a convenience type for storing a list of values,
// sorted by creation date in descendant order.
type History[T AnyCreated[T]] []T

// Len returns the number of values in the history.
func (history *History[T]) Len() int {
	return len(*history)
}

// Less compares two values and returns true if the second one
// was created before the first one.
func (history *History[T]) Less(i, j int) bool {
	values := *history

	return values[j].CreatedAt().Before(values[i].CreatedAt())
}

// Swap switches values i and j positions in the history.
func (history *History[T]) Swap(i, j int) {
	values := *history
	values[i], values[j] = values[j], values[i]
}

// sort orders the history by creation date in descendant order.
func (history *History[T]) sort() {
	sort.Sort(history)
}
