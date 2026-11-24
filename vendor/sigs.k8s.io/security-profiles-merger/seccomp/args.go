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

package seccomp

import (
	"cmp"
	"iter"
	"math"
	"slices"
	"strconv"
	"strings"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// canonicalArg returns the condition as a runtime evaluates it: libseccomp
// reads valueTwo only for SCMP_CMP_MASKED_EQ, so it is cleared for every
// other operator, and for SCMP_CMP_MASKED_EQ it masks valueTwo with the mask
// in value before comparing, so bits outside the mask are cleared. Without
// this, conditions that differ only in ignored bits would be treated as
// different filters.
func canonicalArg(arg specs.LinuxSeccompArg) specs.LinuxSeccompArg {
	if arg.Op == specs.OpMaskedEqual {
		arg.ValueTwo &= arg.Value
	} else {
		arg.ValueTwo = 0
	}

	return arg
}

// tautology reports whether a canonical condition holds for every value.
// libseccomp drops such a condition from its rule: a SCMP_CMP_MASKED_EQ
// with an empty mask.
func tautology(arg specs.LinuxSeccompArg) bool {
	return arg.Op == specs.OpMaskedEqual && arg.Value == 0
}

// sortedArgs returns a sorted, canonical copy of the argument filters.
func sortedArgs(args []specs.LinuxSeccompArg) []specs.LinuxSeccompArg {
	if len(args) == 0 {
		return nil
	}

	cloned := make([]specs.LinuxSeccompArg, len(args))
	for idx, arg := range args {
		cloned[idx] = canonicalArg(arg)
	}

	slices.SortFunc(cloned, compareArg)

	return cloned
}

// compareArg orders two conditions by index, value, valueTwo and operator.
func compareArg(left, right specs.LinuxSeccompArg) int {
	return cmp.Or(
		cmp.Compare(left.Index, right.Index),
		cmp.Compare(left.Value, right.Value),
		cmp.Compare(left.ValueTwo, right.ValueTwo),
		cmp.Compare(left.Op, right.Op),
	)
}

// argKeyBytes sizes the key builder for a typical condition, which runs to
// about eighteen bytes ("0:SCMP_CMP_EQ:0:0;"). It is a hint, not a bound. It
// stays small because Builder.String keeps the whole buffer alive for as
// long as the key, and the keys sit in maps for the length of a merge.
const argKeyBytes = 24

// sortedArgsKey formats args that are already sorted and canonical, as
// clause args always are, without copying them first. It is the hot path of
// the merge: every clause comparison and grouping goes through it.
func sortedArgsKey(args []specs.LinuxSeccompArg) string {
	if len(args) == 0 {
		return ""
	}

	var builder strings.Builder

	builder.Grow(len(args) * argKeyBytes)
	writeArgsKey(&builder, args)

	return builder.String()
}

// decimalBase is the base keys spell numbers in.
const decimalBase = 10

// writeArgsKey writes the key sortedArgsKey returns to builder. The numbers
// are formatted into a buffer on the stack rather than into a string each.
//
// The operator is escaped (see writeEscaped): Diff validates nothing, so
// an operator may hold any byte, and spelled bare, one holding the
// separators would spell two conditions, as "SCMP_CMP_EQ:1:0;1:SCMP_CMP_EQ"
// against 2 spells a0 == 1 && a1 == 2, which made two different rules one.
// Escaped, every key reads back as one list of conditions, while a known
// operator, which holds no separator, is written as it is, so the keys of
// valid conditions, which also order the clauses of a result, are the same
// as unescaped.
func writeArgsKey(builder *strings.Builder, args []specs.LinuxSeccompArg) {
	var digits [20]byte

	for _, arg := range args {
		builder.Write(strconv.AppendUint(digits[:0], uint64(arg.Index), decimalBase))
		builder.WriteByte(':')
		writeEscaped(builder, string(arg.Op), argKeySpecials)
		builder.WriteByte(':')
		builder.Write(strconv.AppendUint(digits[:0], arg.Value, decimalBase))
		builder.WriteByte(':')
		builder.Write(strconv.AppendUint(digits[:0], arg.ValueTwo, decimalBase))
		builder.WriteByte(';')
	}
}

// argKeySpecials are the bytes writeArgsKey escapes in an operator: its
// separators and the escape itself.
const argKeySpecials = `\:;`

// writeEscaped writes text with a backslash before every byte of specials,
// which holds the backslash itself, so that a key holding it reads back
// the same way whatever bytes it holds: the text ends at the first special
// byte without a backslash before it. Text without a special byte, which is
// every known operator and action, is written as it is.
func writeEscaped(builder *strings.Builder, text, specials string) {
	if !strings.ContainsAny(text, specials) {
		builder.WriteString(text)

		return
	}

	for idx := range len(text) {
		if strings.IndexByte(specials, text[idx]) >= 0 {
			builder.WriteByte('\\')
		}

		builder.WriteByte(text[idx])
	}
}

// conjoinArgs returns a single filter matching exactly the calls matched by
// both inputs. OCI argument filters are AND-joined and a runtime accepts at
// most one condition per argument index, so the conjunction exists only when
// every index present on both sides carries one condition that says what
// both say: the same condition, or one of two where the one implies the
// other (see condImplies), which is then the conjunction of the two. The
// second return value is false when no such filter exists.
//
// The conditions on one index are a run in sorted input, and the runs are
// merged in index order (see indexRuns) into a result that is sorted too.
// Clause args are always sorted; an input that is not, which would split a
// run and compare half of it, is sorted into a copy first, so the result
// never depends on the order a caller passes.
func conjoinArgs(
	left, right []specs.LinuxSeccompArg,
) ([]specs.LinuxSeccompArg, bool) {
	left, right = sortedForRuns(left), sortedForRuns(right)
	result := make([]specs.LinuxSeccompArg, 0, len(left)+len(right))

	for leftRun, rightRun := range indexRuns(left, right) {
		switch {
		case len(rightRun) == 0:
			result = append(result, leftRun...)
		case len(leftRun) == 0:
			result = append(result, rightRun...)
		default:
			stronger, ok := strongerGroup(leftRun, rightRun)
			if !ok {
				return nil, false
			}

			result = append(result, stronger...)
		}
	}

	return result, true
}

// sortedForRuns returns args as they are when they are sorted, which costs
// one pass, and a sorted copy otherwise. Unlike byIndex it orders the
// conditions within an index too, which strongerGroup compares as lists.
func sortedForRuns(args []specs.LinuxSeccompArg) []specs.LinuxSeccompArg {
	if slices.IsSortedFunc(args, compareArg) {
		return args
	}

	sorted := slices.Clone(args)
	slices.SortFunc(sorted, compareArg)

	return sorted
}

// strongerGroup returns the conditions of one argument index that match
// exactly the calls both groups match: either group when they are equal, or
// the single condition that implies the single condition of the other. A
// group holds several conditions only in unvalidated input, where nothing
// but equality is read from it.
func strongerGroup(left, right []specs.LinuxSeccompArg) ([]specs.LinuxSeccompArg, bool) {
	switch {
	case slices.Equal(left, right):
		return left, true
	case len(left) != 1 || len(right) != 1:
		return nil, false
	case condImplies(left[0], right[0]):
		return left, true
	case condImplies(right[0], left[0]):
		return right, true
	default:
		return nil, false
	}
}

// argsCover reports whether the filter wide matches every call the filter
// narrow matches: each condition of wide is implied by a condition narrow
// puts on the same argument. A filter whose conditions all appear in the
// other covers it, as does one whose conditions say less: "a0 != 40" covers
// "a0 == 2". It is conservative: false means wide may still cover narrow.
//
// The filters of a clause are sorted by argument index (see loadedArgs), so
// the two are walked side by side and only conditions on one index are
// compared, which keeps a bare list whose clauses carry thousands of
// conditions linear rather than quadratic. A filter in another order is
// sorted first.
func argsCover(wide, narrow []specs.LinuxSeccompArg) bool {
	for wideRun, narrowRun := range indexRuns(byIndex(wide), byIndex(narrow)) {
		for _, loose := range wideRun {
			if !slices.ContainsFunc(narrowRun, func(strict specs.LinuxSeccompArg) bool {
				return condImplies(strict, loose)
			}) {
				return false
			}
		}
	}

	return true
}

// byIndex returns the conditions sorted by argument index: as they are,
// which is how a clause holds them (see loadedArgs), or else a sorted copy.
func byIndex(args []specs.LinuxSeccompArg) []specs.LinuxSeccompArg {
	compare := func(left, right specs.LinuxSeccompArg) int {
		return cmp.Compare(left.Index, right.Index)
	}

	if slices.IsSortedFunc(args, compare) {
		return args
	}

	sorted := slices.Clone(args)
	slices.SortStableFunc(sorted, compare)

	return sorted
}

// indexRuns walks two filters sorted by argument index side by side and
// yields, for every index either one tests, the conditions each puts on it,
// one of which is empty where only the other tests the index.
func indexRuns(
	left, right []specs.LinuxSeccompArg,
) iter.Seq2[[]specs.LinuxSeccompArg, []specs.LinuxSeccompArg] {
	return func(yield func(left, right []specs.LinuxSeccompArg) bool) {
		for len(left) > 0 || len(right) > 0 {
			var index uint

			switch {
			case len(left) == 0:
				index = right[0].Index
			case len(right) == 0:
				index = left[0].Index
			default:
				index = min(left[0].Index, right[0].Index)
			}

			leftRun, rightRun := runOf(left, index), runOf(right, index)
			left, right = left[len(leftRun):], right[len(rightRun):]

			if !yield(leftRun, rightRun) {
				return
			}
		}
	}
}

// runOf returns the conditions at the start of args that test index.
func runOf(args []specs.LinuxSeccompArg, index uint) []specs.LinuxSeccompArg {
	end := 0
	for end < len(args) && args[end].Index == index {
		end++
	}

	return args[:end]
}

// condImplies reports whether every value the condition strong matches is
// one the condition weak matches, both read on one argument. It is
// conservative: false means the implication may still hold.
//
// Two different conditions are compared only when neither holds a value
// above 32 bits. libseccomp compares the lower 32 bits alone on a 32-bit
// architecture, where "a0 == 0x100000028" matches 40 and so does not imply
// "a0 != 40", and below that width a condition reads the same on every
// architecture.
func condImplies(strong, weak specs.LinuxSeccompArg) bool {
	if strong == weak {
		return true
	}

	if wideArg(strong) || wideArg(weak) {
		return false
	}

	switch strong.Op {
	case specs.OpEqualTo:
		return condHolds(weak, strong.Value)
	case specs.OpMaskedEqual:
		return maskImplies(strong, weak)
	case specs.OpNotEqual, specs.OpLessThan, specs.OpLessEqual,
		specs.OpGreaterEqual, specs.OpGreaterThan:
		return rangeImplies(strong, weak)
	default:
		return false
	}
}

// rangeImplies is condImplies for a condition that matches a range of
// values, if any: it implies a range holding its own, and that the argument
// is not a value outside it. A range of one value, "a0 < 1" or "a0 <= 0",
// is that value spelled another way, and implies what "a0 == 0" implies:
// read apart, a filter seen to cover "a0 == 0" but not "a0 < 1" made a merge
// of three inputs depend on their order.
func rangeImplies(strong, weak specs.LinuxSeccompArg) bool {
	low, high, ok := condInterval(strong)
	if !ok {
		return false
	}

	if low == high {
		return condHolds(weak, low)
	}

	if weak.Op == specs.OpNotEqual {
		return weak.Value < low || weak.Value > high
	}

	weakLow, weakHigh, weakOk := condInterval(weak)

	return weakOk && weakLow <= low && high <= weakHigh
}

// maskImplies is condImplies for a masked comparison: it fixes the bits of
// its mask, so it implies a masked comparison of a part of those bits with
// the same values, and that the argument is not a value that differs from
// its own in one of them.
func maskImplies(strong, weak specs.LinuxSeccompArg) bool {
	switch weak.Op {
	case specs.OpMaskedEqual:
		return weak.Value&^strong.Value == 0 && strong.ValueTwo&weak.Value == weak.ValueTwo
	case specs.OpNotEqual:
		return weak.Value&strong.Value != strong.ValueTwo
	case specs.OpEqualTo, specs.OpLessThan, specs.OpLessEqual,
		specs.OpGreaterEqual, specs.OpGreaterThan:
		return false
	default:
		return false
	}
}

// argsDisjoint reports whether two filters provably never match the same
// call. It is conservative: false means the filters may overlap. It walks
// the filters side by side, as argsCover does.
func argsDisjoint(left, right []specs.LinuxSeccompArg) bool {
	for leftRun, rightRun := range indexRuns(byIndex(left), byIndex(right)) {
		for _, leftArg := range leftRun {
			if slices.ContainsFunc(rightRun, func(rightArg specs.LinuxSeccompArg) bool {
				return condsDisjoint(leftArg, rightArg)
			}) {
				return true
			}
		}
	}

	return false
}

// condsDisjoint reports whether two conditions on the same argument can never
// both hold.
func condsDisjoint(left, right specs.LinuxSeccompArg) bool {
	if left.Op == specs.OpEqualTo {
		return !condHolds(right, left.Value)
	}

	if right.Op == specs.OpEqualTo {
		return !condHolds(left, right.Value)
	}

	leftLo, leftHi, leftOk := condInterval(left)
	rightLo, rightHi, rightOk := condInterval(right)

	if !leftOk || !rightOk {
		return false
	}

	return leftHi < rightLo || rightHi < leftLo
}

// condInterval returns the inclusive value range (low, high) matched by a
// comparison operator. The third result is false for operators that do not
// describe a contiguous range, or for ranges that are empty.
func condInterval(arg specs.LinuxSeccompArg) (uint64, uint64, bool) {
	switch arg.Op {
	case specs.OpLessThan:
		if arg.Value == 0 {
			return 0, 0, false
		}

		return 0, arg.Value - 1, true
	case specs.OpLessEqual:
		return 0, arg.Value, true
	case specs.OpGreaterThan:
		if arg.Value == math.MaxUint64 {
			return 0, 0, false
		}

		return arg.Value + 1, math.MaxUint64, true
	case specs.OpGreaterEqual:
		return arg.Value, math.MaxUint64, true
	case specs.OpEqualTo:
		return arg.Value, arg.Value, true
	case specs.OpNotEqual, specs.OpMaskedEqual:
		return 0, 0, false
	default:
		return 0, 0, false
	}
}

// condHolds evaluates a single argument condition against a concrete value
// the way the program libseccomp compiles for a 64-bit architecture does.
// Unknown operators never hold.
func condHolds(arg specs.LinuxSeccompArg, value uint64) bool {
	switch arg.Op {
	case specs.OpNotEqual:
		return value != arg.Value
	case specs.OpLessThan:
		return value < arg.Value
	case specs.OpLessEqual:
		return value <= arg.Value
	case specs.OpEqualTo:
		return value == arg.Value
	case specs.OpGreaterEqual:
		return value >= arg.Value
	case specs.OpGreaterThan:
		return value > arg.Value
	case specs.OpMaskedEqual:
		// libseccomp masks the datum as well, see canonicalArg.
		return value&arg.Value == arg.ValueTwo&arg.Value
	default:
		return false
	}
}
