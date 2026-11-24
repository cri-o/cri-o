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

// Package spm holds the declarations the seccomp, apparmor and landlock
// packages share. Each of them re-exports what it uses under its own name
// apart from [Problems]; import this package for Problems, or to match a
// sentinel without picking one of the three.
//
//   - [SliceDiff] and [ValueDiff] are the shared diff shapes:
//     seccomp.SliceDiff, apparmor.SliceDiff and landlock.SliceDiff are
//     aliases of SliceDiff, and seccomp.ValueDiff and apparmor.ValueDiff
//     of ValueDiff, so a value of one is a value of the others.
//   - [InputError] and the sentinel errors are the same variables in every
//     package that re-exports them, so errors.Is against one here matches
//     an error any of the three returned.
//   - [Problems] lists the failures an error reports.
//   - [MaxPathLen] is the path length limit of apparmor and landlock.
//
// # Reading a validation error
//
// A validator collects its failures into one error and reports at most 32
// of them. Past that the error matches [ErrMoreProblems] instead of listing
// the rest, so errors.Is for a sentinel the profile violates can be false.
// Treat a match of ErrMoreProblems as "and possibly others" before
// dispatching on another sentinel, and do not branch on the absence of one.
// A merge bounds the InputErrors of its failing inputs the same way. This
// is the one place the bound is described; the packages refer here.
//
// An error holding more than one failure, or having left any out,
// implements Unwrap() []error, which returns the failures it kept followed
// by ErrMoreProblems when it left others out. A single failure is returned
// as it is. [Problems] lists the failures of either kind. InputError wraps
// the report of one input with Unwrap() error, so call Problems on its Err
// to list that input's failures.
//
// # Path checks
//
// apparmor and landlock check a path's length ([ErrPathTooLong]) before
// scanning it, so an oversized path costs no further work. Only the count
// limits of ValidateArtifact and ValidateStrict, and in landlock the
// empty-path check, run before it. apparmor checks the length of every path
// of the profile first and, when one is too long, skips the other checks of
// Validate; landlock checks each path's length before scanning that path.
//
// # Strict decoding
//
// UnmarshalStrict of each package refuses what encoding/json accepts
// silently, since each such document can mean two different profiles to
// two readers, such as a scanner that approved an artifact and the runtime
// that loads it. encoding/json decodes a repeated member into what the one
// before it filled, so a later object merges into the earlier one and a
// later scalar or array replaces it ([ErrDuplicateKey]), while other
// parsers keep the first or the last; drops a member the type has no field
// for ([ErrUnknownField]), losing the rule a misspelling meant to carry;
// fills a field from a member that matches it only ignoring case
// ([ErrMisspelledField]), which a reader comparing names exactly drops;
// decodes a missing member as its zero value ([ErrMissingField]), which for
// a seccomp condition is argument 0, for a Landlock network rule port 0,
// and for a null seccomp syscall entry an entry without names or action;
// replaces invalid UTF-8 with U+FFFD ([ErrInvalidUTF8]), so distinct
// documents decode alike; drops a second value behind the profile
// ([ErrUnexpectedData]); and decodes null as an empty profile
// ([ErrNotAnObject]).
//
// UnmarshalStrict reports the first kind of problem it finds, naming every
// member of that kind up to a bound. Invalid UTF-8 comes before anything
// else, and a value that is not an object before what follows it. A
// document that is otherwise not well-formed JSON gets the decoder's error,
// its offset counted as json.Unmarshal counts it; an empty or truncated one
// gets the *json.SyntaxError json.Unmarshal returns, which matches neither
// io.EOF nor io.ErrUnexpectedEOF. Every message is bounded in length, and
// the profile given is replaced only when decoding succeeds.
//
// # Scope
//
// The three profile types have no common shape, and no operation over "a
// profile" can say anything before it knows which of the three it has, so
// anything beyond these declarations belongs in the package that knows the
// type. The spm command dispatches over a table of per-type functions.
package spm

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"sigs.k8s.io/security-profiles-merger/internal/report"
)

var (
	// ErrNoProfiles is returned by Intersect and Union of every package
	// when no profiles are given.
	ErrNoProfiles = errors.New("at least one profile is required")
	// ErrNilProfile is returned when a profile argument is nil.
	ErrNilProfile = errors.New("profile must not be nil")
	// ErrEmptyPath is returned by apparmor and landlock when a path rule
	// holds an empty string.
	ErrEmptyPath = errors.New("empty path")
	// ErrNulInPath is returned when a path holds a NUL byte: by every
	// landlock validator and merge, and by apparmor's ValidateArtifact and
	// ValidateStrict, which also return it for an escape denoting one.
	ErrNulInPath = errors.New("path contains a NUL byte")
	// ErrRelativePath is returned by apparmor and landlock when a path does
	// not start with "/".
	ErrRelativePath = errors.New("relative path (must be absolute)")
	// ErrDotComponent is matched when a path has a component the package
	// does not resolve: by apparmor's ValidateArtifact, ValidateStrict and
	// ValidateBaseline for a "." or ".." component, which matches nothing,
	// and by every landlock validator and merge for a ".." component, which
	// the kernel resolves against the file system, reported as such
	// (landlock cleans "." components away).
	ErrDotComponent = errors.New(`path contains a "." or ".." component`)
	// ErrPathTooLong is returned by apparmor and landlock when a path is
	// longer than [MaxPathLen] bytes. See the Path checks section for the
	// checks that run before it.
	ErrPathTooLong = errors.New("path exceeds " + strconv.Itoa(MaxPathLen) + " bytes")
	// ErrMoreProblems stands in for the failures a validator's report, or a
	// merge's joined InputErrors, left out. See the Reading a validation
	// error section.
	ErrMoreProblems = errors.New("more problems omitted")
	// ErrBudgetExceeded is returned, never inside an InputError, by
	// apparmor.Intersect, landlock.Intersect and landlock.Union when the
	// merge would run past a bound on its own work, and by
	// apparmor.ValidateBaseline for a baseline whose patterns leave the
	// merge too little compile budget. See the Cost bounds section of each
	// package for what the budgets admit.
	ErrBudgetExceeded = errors.New("merge budget exceeded")
	// ErrInexactBaseline is returned by seccomp.ValidateBaseline and
	// apparmor.ValidateBaseline for a baseline Validate accepts but whose
	// intersection alone, or for seccomp whose intersection with a profile
	// allowing every call, differs from it, so every merge with it takes
	// away the rules the message names. It means a misconfigured node.
	ErrInexactBaseline = errors.New("baseline holds rules the merge does not read exactly")
	// ErrInvalidOptions is matched by every error a method of
	// seccomp.Options or landlock.Options returns for options that name no
	// node: the zero seccomp.Options, a seccomp.ForArch architecture the
	// package does not know (seccomp.ErrUnknownArch), or a landlock.Options
	// version below ABIV1 (landlock.ErrUnknownABIVersion). It is returned
	// before any profile is read and never inside an InputError, so it
	// means a misconfigured node, never a bad profile.
	ErrInvalidOptions = errors.New("invalid options")
	// ErrArtifactTooLarge is matched by every error ValidateArtifact and
	// ValidateStrict of the three packages return for a profile past one of
	// their MaxArtifact limits, which are the ErrTooMany sentinels of each
	// package. It is never returned on its own.
	ErrArtifactTooLarge = errors.New("profile exceeds an artifact limit")

	// ErrDuplicateKey is returned by UnmarshalStrict of every package when
	// a member occurs more than once in one JSON object, compared ignoring
	// case in an object decoding into a struct, as encoding/json matches
	// members to fields, and exactly in any other, such as one inside a
	// member no field reads. See the Strict decoding section for why these
	// documents are refused.
	ErrDuplicateKey = errors.New("duplicate key")
	// ErrUnknownField is returned by UnmarshalStrict of every package when
	// the document holds a member the profile type has no field for.
	ErrUnknownField = errors.New("unknown field")
	// ErrMisspelledField is returned by UnmarshalStrict of every package
	// when a member names a field only ignoring case, such as "Syscalls" or
	// "ſyscalls" for "syscalls".
	ErrMisspelledField = errors.New("misspelled field")
	// ErrMissingField is returned by seccomp.UnmarshalStrict and
	// landlock.UnmarshalStrict when the document leaves out, or sets to
	// null, a member with no value for its absence: the names or action of
	// a seccomp syscall entry, the index or op of a seccomp argument
	// condition, or the path or port of a Landlock rule, also by a null
	// entry, condition or rule.
	ErrMissingField = errors.New("missing field")
	// ErrInvalidUTF8 is returned by UnmarshalStrict of every package when
	// the document holds a byte that is not valid UTF-8, or a \u escape
	// spelling half a surrogate pair.
	ErrInvalidUTF8 = errors.New(
		"invalid UTF-8, which the JSON decoder replaces with U+FFFD, " +
			"so distinct profiles can decode alike",
	)
	// ErrUnexpectedData is returned by UnmarshalStrict of every package
	// when the document is followed by anything but whitespace.
	ErrUnexpectedData = errors.New("unexpected data after the profile")
	// ErrNotAnObject is returned by UnmarshalStrict of every package when
	// the document is not a JSON object, null included.
	ErrNotAnObject = errors.New("not a JSON object")
)

// Problems returns the failures err reports: nil for a nil err, the
// failures of a report a validator or merge of this module returns or of
// an errors.Join, each report or join among them replaced by its own
// failures in turn, and err itself otherwise. ErrMoreProblems comes last
// and once, however many reports held it. Nothing else is flattened: an
// error wrapping a single one, such as an InputError, or one built with
// several %w verbs is one problem, so call Problems on an InputError's Err
// to list that input's failures.
func Problems(err error) []error {
	if err == nil {
		return nil
	}

	var problems []error

	more := false

	var walk func(error)

	walk = func(err error) {
		joined, ok := err.(interface{ Unwrap() []error })
		if !ok || !isReport(err) {
			//nolint:err113,errorlint // the sentinel itself, not an error wrapping it
			if err == ErrMoreProblems {
				more = true
			} else {
				problems = append(problems, err)
			}

			return
		}

		for _, each := range joined.Unwrap() {
			if each != nil {
				walk(each)
			}
		}
	}

	walk(err)

	if more {
		problems = append(problems, ErrMoreProblems)
	}

	return problems
}

// joinType is the type errors.Join returns.
var joinType = reflect.TypeOf(errors.Join(ErrMoreProblems))

// isReport reports whether err lists failures of its own: the report a
// validator or merge of this module returns, or a join errors.Join
// returned. Only those two types count, so no method of another error
// makes Problems take it apart.
func isReport(err error) bool {
	//nolint:errorlint // the report itself, not one wrapped
	if _, ok := err.(*report.ListError); ok {
		return true
	}

	return reflect.TypeOf(err) == joinType
}

// InputError names the one input profile an error is about. Intersect and
// Union of every package, and landlock.IntersectLowered, return one for each
// input that is nil or fails validation, and a merge also for an error its
// package documents as brought by one input, such as
// seccomp.ErrConflictingListener; Diff returns one for the first nil
// profile only, so Diff(nil, nil) names Index 0. Several are joined in
// input order and bounded as a validator bounds its failures (see the
// Reading a validation error section); list them with [Problems], or match
// the first with errors.As. errors.Is sees through it to the sentinels of
// Err.
//
// Errors no single input caused come without an InputError: ErrNoProfiles,
// ErrBudgetExceeded, and the errors each package documents for its result,
// such as seccomp.ErrLostRules.
type InputError struct {
	// Index is the zero-based position of the profile among the inputs:
	// for a merge 0 is the first profile, where a runtime passes its
	// baseline; for Diff 0 is left and 1 is right.
	Index int
	// Err is what was wrong with the profile: ErrNilProfile, the error of
	// the package's Validate, which may report several failures, or another
	// error the package documents as naming one input. Match it with
	// errors.Is rather than by comparison.
	Err error
}

// Error returns "profile <Index>: " followed by the message of Err.
func (e *InputError) Error() string {
	return fmt.Sprintf("profile %d: %v", e.Index, e.Err)
}

// Unwrap returns Err, so that errors.Is and errors.As see the sentinels it
// holds.
func (e *InputError) Unwrap() error { return e.Err }

// MaxPathLen is the longest path, in bytes, the apparmor and landlock
// packages accept: Linux's PATH_MAX of 4096 less its terminating NUL.
const MaxPathLen = 4095

// SliceDiff holds the items added and removed between two lists compared
// as sets, ignoring order and repeats. A diff field of this type is nil
// when the lists are equal, so a non-nil SliceDiff holds at least one item.
type SliceDiff[T comparable] struct {
	// Added holds the items only the right profile lists, sorted.
	Added []T `json:"added,omitempty"`
	// Removed holds the items only the left profile lists, sorted.
	Removed []T `json:"removed,omitempty"`
}

// ValueDiff holds a changed value. A diff field of this type is nil when
// the two values are equal; the field says what a side holds where its
// profile sets no value.
type ValueDiff[T any] struct {
	// Left is the value of the left profile.
	Left T `json:"left"`
	// Right is the value of the right profile.
	Right T `json:"right"`
}
