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

// Package report holds the error type that lists several failures, which
// merge.Collector builds and spm.Problems takes apart. It imports nothing of
// this module, so that both can name the type: spm.Problems flattens this
// type and the joins errors.Join builds, and no other error, whatever
// methods it has.
package report

import (
	"fmt"
	"slices"
	"strings"
)

// ListError is a list of failures and a count of the ones left out. more is
// the sentinel that stands for those, spm.ErrMoreProblems, which this
// package cannot import.
type ListError struct {
	kept    []error
	omitted int
	more    error
}

// New returns the report of the kept failures and the count of the omitted
// ones, which more stands for. It keeps a copy of kept.
func New(kept []error, omitted int, more error) *ListError {
	return &ListError{kept: slices.Clone(kept), omitted: omitted, more: more}
}

// Kept returns the failures the report lists. The caller must not modify
// the slice.
func (r *ListError) Kept() []error { return r.kept }

// Omitted returns how many failures the report left out.
func (r *ListError) Omitted() int { return r.omitted }

// Error lists the kept failures one per line, as errors.Join does, followed
// by the count of the omitted ones.
func (r *ListError) Error() string {
	lines := make([]string, 0, len(r.kept)+1)
	for _, err := range r.kept {
		lines = append(lines, err.Error())
	}

	if r.omitted > 0 {
		lines = append(lines, fmt.Sprintf("%d %v", r.omitted, r.more))
	}

	return strings.Join(lines, "\n")
}

// Unwrap returns the kept failures, and the sentinel for the omitted ones
// when any were left out, so that errors.Is sees both.
func (r *ListError) Unwrap() []error {
	if r.omitted == 0 {
		return r.kept
	}

	return append(slices.Clone(r.kept), r.more)
}
