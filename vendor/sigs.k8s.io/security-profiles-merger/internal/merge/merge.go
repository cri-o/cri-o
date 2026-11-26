/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package merge holds what the three profile packages and the command share
// beyond the public spm package: the generic merge primitives (Fold and the
// slice helpers), the bounds on error reports (Collector, BoundedError,
// QuoteBounded), the quoting of profile values for output (Builder.WriteText,
// SafeName) and the bounded output of the formatters (Builder).
package merge

import (
	"cmp"
	"slices"

	"sigs.k8s.io/security-profiles-merger/spm"
)

// Fold merges a slice of profiles left to right with mergeFn, stopping at
// the first error it returns. A single profile is returned as it is, so a
// caller that must not hand back its input copies it. An empty slice
// reports spm.ErrNoProfiles. Profiles must be non-nil: callers validate
// them, reporting ErrNilProfile with the index, before folding.
func Fold[T any](profiles []*T, mergeFn func(*T, *T) (*T, error)) (*T, error) {
	if len(profiles) == 0 {
		return nil, spm.ErrNoProfiles
	}

	result := profiles[0]

	for _, profile := range profiles[1:] {
		var err error

		result, err = mergeFn(result, profile)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// RequireNonNil reports the first nil profile as an *spm.InputError naming
// its position and wrapping spm.ErrNilProfile, or returns nil when there is
// none. It is the check every Diff runs on its two inputs, so that a nil
// one is reported as the merges report it.
func RequireNonNil[T any](profiles ...*T) error {
	for idx, profile := range profiles {
		if profile == nil {
			return &spm.InputError{Index: idx, Err: spm.ErrNilProfile}
		}
	}

	return nil
}

// SortedKeyDiff compares the keys of two maps and returns, in this order and
// each sorted, the keys only right has (added), those only left has
// (removed) and those both have. A list without keys is nil.
func SortedKeyDiff[K cmp.Ordered, V any](left, right map[K]V) ([]K, []K, []K) {
	var added, removed, both []K

	for key := range left {
		if _, ok := right[key]; ok {
			both = append(both, key)
		} else {
			removed = append(removed, key)
		}
	}

	for key := range right {
		if _, ok := left[key]; !ok {
			added = append(added, key)
		}
	}

	slices.Sort(added)
	slices.Sort(removed)
	slices.Sort(both)

	return added, removed, both
}

// NilIfEmpty returns nil for an empty slice and the slice itself otherwise,
// so that a result left without items spells its list as absent.
func NilIfEmpty[T any](items []T) []T {
	if len(items) == 0 {
		return nil
	}

	return items
}

// NewSliceDiff compares two slices as sets and returns the elements only
// right has (added) and those only left has (removed), each sorted, which
// is why T must be ordered. Duplicates within a slice are ignored. It
// returns nil when the sets are equal, which is how every diff of this
// module reports an unchanged list.
func NewSliceDiff[T cmp.Ordered](left, right []T) *spm.SliceDiff[T] {
	leftSet := make(map[T]struct{}, len(left))
	for _, item := range left {
		leftSet[item] = struct{}{}
	}

	rightSet := make(map[T]struct{}, len(right))
	for _, item := range right {
		rightSet[item] = struct{}{}
	}

	added, removed, _ := SortedKeyDiff(leftSet, rightSet)
	if len(added) == 0 && len(removed) == 0 {
		return nil
	}

	return &spm.SliceDiff[T]{Added: added, Removed: removed}
}

// NewValueDiff returns the two values as a ValueDiff, or nil when they are
// equal, which is how every diff of this module reports an unchanged value.
func NewValueDiff[T comparable](left, right T) *spm.ValueDiff[T] {
	if left == right {
		return nil
	}

	return &spm.ValueDiff[T]{Left: left, Right: right}
}

// IntersectSlice returns the elements present in both left and right, each
// once, in the order left holds them, or nil when there are none, whether
// a side is empty or the two are disjoint.
func IntersectSlice[T comparable](left, right []T) []T {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}

	rightSet := make(map[T]struct{}, len(right))
	for _, val := range right {
		rightSet[val] = struct{}{}
	}

	result := make([]T, 0, min(len(left), len(right)))
	seen := make(map[T]struct{}, len(left))

	for _, val := range left {
		if _, ok := rightSet[val]; !ok {
			continue
		}

		if _, dup := seen[val]; !dup {
			seen[val] = struct{}{}
			result = append(result, val)
		}
	}

	return NilIfEmpty(result)
}

// UnionSlice returns the elements of left and then of right, each once.
//
// The lists it is given are rights, architectures and flags: a handful of
// elements, merged once per rule and per ancestor of a rule, so it is the
// allocations that cost rather than the comparisons. A linear scan of the
// result needs no map, and past a few elements the map takes over so that a
// list a profile chose the length of stays linear.
func UnionSlice[T comparable](left, right []T) []T {
	if len(left)+len(right) == 0 {
		return nil
	}

	if len(left)+len(right) > smallUnion {
		return DeduplicateSlice(slices.Concat(left, right))
	}

	result := make([]T, 0, len(left)+len(right))

	for _, list := range [2][]T{left, right} {
		for _, val := range list {
			if !slices.Contains(result, val) {
				result = append(result, val)
			}
		}
	}

	return result
}

// smallUnion is the combined length up to which UnionSlice scans instead of
// hashing.
const smallUnion = 16

// ClonePtr returns a shallow copy of the pointed-to value, or nil if ptr is nil.
func ClonePtr[T any](ptr *T) *T {
	if ptr == nil {
		return nil
	}

	val := *ptr

	return &val
}

// DeduplicateSlice returns a new slice with duplicate elements removed,
// preserving the order of first occurrence. The result never shares a backing
// array with items: an empty input yields nil rather than the caller's slice,
// so appending to the result cannot write into the caller's array.
func DeduplicateSlice[T comparable](items []T) []T {
	if len(items) == 0 {
		return nil
	}

	seen := make(map[T]struct{}, len(items))
	result := make([]T, 0, len(items))

	for _, item := range items {
		if _, ok := seen[item]; !ok {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}

	return result
}
