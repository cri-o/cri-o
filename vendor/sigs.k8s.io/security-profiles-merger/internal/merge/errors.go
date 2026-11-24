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

package merge

import (
	"fmt"
	"unicode/utf8"

	"sigs.k8s.io/security-profiles-merger/internal/report"
	"sigs.k8s.io/security-profiles-merger/spm"
)

// MaxQuotedBytes bounds how much of a caller-supplied value QuoteBounded
// renders. A validation error names the input that failed, and for an
// artifact that input is attacker-controlled and unbounded, so a runtime
// logging the error would otherwise write the artifact back out at whatever
// size its author chose.
const MaxQuotedBytes = 64

// MaxJoinedErrors bounds how many errors a Collector reports individually.
// Validation collects every failure, and a profile can hold as many failures
// as it holds entries, so the count needs a ceiling for the same reason the
// individual values do.
const MaxJoinedErrors = 32

// QuoteBounded renders value as a double-quoted Go string, truncated to
// MaxQuotedBytes on a rune boundary. A truncated value is followed by an
// ellipsis outside the quotes, so the elision is never mistaken for part of
// the value. Use it instead of %q wherever the value comes from a profile
// rather than from this package.
func QuoteBounded(value string) string {
	if len(value) <= MaxQuotedBytes {
		return quoteVisible(value)
	}

	return quoteVisible(value[:runeBoundary(value, MaxQuotedBytes)]) + "..."
}

// runeBoundary returns the largest index up to limit at which text can be
// cut without splitting a UTF-8 sequence. A sequence is at most utf8.UTFMax
// bytes long, so the cut backs off by less than that: text that holds no
// rune start within reach is not valid UTF-8, where there is no sequence to
// keep whole and backing off further would cut away what is to be shown.
func runeBoundary(text string, limit int) int {
	for back := range utf8.UTFMax {
		if utf8.RuneStart(text[limit-back]) {
			return limit - back
		}
	}

	return limit
}

// maxMessageBytes bounds a message built from text the caller did not
// write, such as the literal a JSON decoder quotes back from the document
// it refused.
const maxMessageBytes = 512

// BoundedText truncates text to maxMessageBytes on a rune boundary, marking
// the elision so that it is never mistaken for the text. A decoder quotes
// the literal it refused, and a document chooses how long that is: a
// megabyte-long number otherwise reaches a runtime's log as a megabyte of
// error.
func BoundedText(text string) string {
	if len(text) <= maxMessageBytes {
		return text
	}

	return text[:runeBoundary(text, maxMessageBytes)] + "..."
}

// BoundedError returns err with its message bounded by BoundedText. errors.Is
// and errors.As still see err itself, so a caller matching a decoder's error
// type is unaffected. It returns nil for a nil err.
func BoundedError(err error) error {
	if err == nil {
		return nil
	}

	return &boundedError{err: err}
}

// boundedError is the result of BoundedError.
type boundedError struct {
	err error
}

func (b *boundedError) Error() string { return BoundedText(b.err.Error()) }

func (b *boundedError) Unwrap() error { return b.err }

// Collector gathers up to MaxJoinedErrors failures into one report,
// followed by a count of the ones it left out. Report builds a failure only
// while the report has room for it: a profile with a million bad entries
// would otherwise cost a million formatted messages for a report that shows
// 32 of them. The zero value is ready to use.
//
// The omitted failures stay omitted: a caller that needs to match one with
// errors.Is must not rely on a failure past the limit being present. A
// truncated report matches spm.ErrMoreProblems, so the truncation itself is
// visible rather than silent.
type Collector struct {
	kept    []error
	omitted int
}

// Report adds the failure build returns, calling build only when the report
// still has room for it. Past the limit the failure is counted as omitted
// without being built, so Report is for a check that has already failed:
// build must return a non-nil error and have no other effect.
func (c *Collector) Report(build func() error) {
	if !c.full() {
		c.Add(build())
	}
}

// Reportf adds the failure fmt.Errorf builds from format and args, building
// it only when the report still has room for it, as Report does. The
// arguments are evaluated and boxed even past the limit, so Reportf is for a
// check that fails a bounded number of times; one that can fail once per
// element of its input uses Report, which costs nothing past the limit.
func (c *Collector) Reportf(format string, args ...any) {
	if !c.full() {
		//nolint:err113 // the callers' formats wrap their sentinels with %w
		c.Add(fmt.Errorf(format, args...))
	}
}

// Add adds err. A report from Collector.Err is flattened into this one
// rather than counted as one failure: validators build their report from the
// reports of their checks, and the bound has to hold for the whole of it,
// not once per check. A nil err is ignored.
func (c *Collector) Add(err error) {
	if err == nil {
		return
	}

	//nolint:errorlint // only a report itself is flattened, not one wrapped
	if nested, ok := err.(*report.ListError); ok {
		for _, kept := range nested.Kept() {
			c.Add(kept)
		}

		c.omitted += nested.Omitted()

		return
	}

	if !c.full() {
		c.kept = append(c.kept, err)
	}
}

// Err returns the collected report: nil when nothing failed, as
// errors.Join returns, the failure itself when exactly one did and nothing
// was omitted, and otherwise a report that also matches spm.ErrMoreProblems
// when it left failures out.
func (c *Collector) Err() error {
	switch {
	case len(c.kept) == 0 && c.omitted == 0:
		return nil
	case len(c.kept) == 1 && c.omitted == 0:
		return c.kept[0]
	default:
		return report.New(c.kept, c.omitted, spm.ErrMoreProblems)
	}
}

// full reports whether the report has no room for another failure, and
// then counts that failure as omitted.
func (c *Collector) full() bool {
	if len(c.kept) < MaxJoinedErrors {
		return false
	}

	c.omitted++

	return true
}

// ReportDuplicates reports each of count items whose key an earlier item
// has. An item without a key, for which key reports false, is skipped.
// duplicate builds the failure for the item at idx, given the index of the
// first item with the same key, and is called only while the report has
// room for it (see Collector.Report).
//
// The set of keys seen starts empty rather than sized for count: a list
// that repeats one key many times holds few distinct ones.
func ReportDuplicates[Key comparable](
	collector *Collector, count int, key func(idx int) (Key, bool),
	duplicate func(idx, first int) error,
) {
	seen := make(map[Key]int)

	for idx := range count {
		itemKey, ok := key(idx)
		if !ok {
			continue
		}

		first, dup := seen[itemKey]
		if !dup {
			seen[itemKey] = idx

			continue
		}

		collector.Report(func() error { return duplicate(idx, first) })
	}
}

// ReportDuplicateValues reports each value of the list at field whose key
// an earlier value has, as in `field[3]: "x": <sentinel> (also at
// field[1])`, quoting the value with QuoteBounded. key returns the key of
// the value at idx.
//
// field is a string or a fmt.Stringer, formatted only for a failure the
// report has room for, so a caller validating many lists can pass a
// Stringer that builds the name then rather than a string per list.
func ReportDuplicateValues[T ~string, Key comparable, Field any](
	collector *Collector, field Field, values []T, key func(idx int) Key, sentinel error,
) {
	ReportDuplicates(collector, len(values),
		func(idx int) (Key, bool) { return key(idx), true },
		func(idx, first int) error {
			return fmt.Errorf(
				"%v[%d]: %s: %w (also at %v[%d])",
				field, idx, QuoteBounded(string(values[idx])), sentinel, field, first,
			)
		})
}

// CheckInputs runs check on every profile, in order, and returns the
// failures as one error, each an *spm.InputError naming the profile's
// index: nil when every profile passed, the InputError itself when one
// failed, and otherwise the InputErrors in input order, joined and bounded
// as a Collector joins and bounds them. errors.As finds the first, and a
// caller unwrapping the result with Unwrap() []error reaches every one
// kept; past MaxJoinedErrors failing profiles the result matches
// spm.ErrMoreProblems.
func CheckInputs[T any](profiles []*T, check func(idx int, profile *T) error) error {
	var collector Collector

	for idx, profile := range profiles {
		err := check(idx, profile)
		if err != nil {
			collector.Report(func() error { return &spm.InputError{Index: idx, Err: err} })
		}
	}

	return collector.Err()
}

// SoleInputError returns the error a merge of one input returned without
// the InputError naming that input, which says nothing to a caller that
// passed one profile, or err itself when it is no InputError.
func SoleInputError(err error) error {
	//nolint:errorlint // only the InputError the merge itself returned
	inputErr, ok := err.(*spm.InputError)
	if ok && inputErr.Index == 0 {
		return inputErr.Err
	}

	return err
}
