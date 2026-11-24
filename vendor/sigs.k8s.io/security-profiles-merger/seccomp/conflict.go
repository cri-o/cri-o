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
	"math"
	"slices"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// conflictTracker remembers the rules seen for one syscall, each as the key
// of its filter and the number of its result. Each condition is numbered
// once per syscall (see filterKey), and each result once (see resultOf), so
// comparing two rules compares a few small numbers. A syscall whose rules
// are checked loads at most MaxArtifactRulesPerSyscall of them, the two
// names of an alias libseccomp reads as one syscall counting as one (see
// syscallAlias), so comparing each with every rule before it costs some 32
// thousand comparisons per syscall and tracker, twice that with the tracker
// for a 32-bit architecture, and up to some two million for a profile at
// MaxArtifactRules.
type conflictTracker struct {
	conditions map[condition]uint16
	results    []clause
	rules      []trackedRule
}

// trackedRule is a rule a conflictTracker has seen.
type trackedRule struct {
	key    filterKey
	result uint16
}

// condition is one condition of a filter without its argument index, which
// its position in a filterKey names.
type condition struct {
	value    uint64
	valueTwo uint64
	op       uint8
}

// argSlots is how many argument indices a condition can test.
const argSlots = maxSyscallArgIndex + 1

// filterKey is a rule's filter as the number of the condition on each
// argument index, or zero where the index carries none. A rule of a profile
// that passes Validate has at most one condition per index, so every filter
// of one has a key. Two filters of one syscall have the same key exactly
// when sortedArgsKey spells them alike.
type filterKey [argSlots]uint16

// nested reports whether one of the filters k and other is equal to or
// wider than the other: every condition of one is one of the other's.
func (k filterKey) nested(other filterKey) bool {
	kWider, otherWider := true, true

	for idx := range argSlots {
		if k[idx] == other[idx] {
			continue
		}

		kWider = kWider && k[idx] == 0
		otherWider = otherWider && other[idx] == 0

		if !kWider && !otherWider {
			return false
		}
	}

	return true
}

func newConflictTracker() *conflictTracker {
	return &conflictTracker{conditions: make(map[condition]uint16)}
}

// keyOf returns the key of a rule's filter, numbering its conditions, or
// false where the rule has a condition on an index past maxSyscallArgIndex,
// two on one index or an unknown operator, which no rule of a profile
// passing Validate has, or where the syscall has more distinct conditions
// than a key can number, which MaxArtifactRulesPerSyscall rules of six
// conditions each do not come near.
func (t *conflictTracker) keyOf(args []specs.LinuxSeccompArg) (filterKey, bool) {
	var key filterKey

	for _, arg := range args {
		op := operatorCode(arg.Op)
		if arg.Index > maxSyscallArgIndex || key[arg.Index] != 0 || op == opUnknown {
			return key, false
		}

		cond := condition{value: arg.Value, valueTwo: arg.ValueTwo, op: op}

		number, ok := t.conditions[cond]
		if !ok {
			if len(t.conditions) >= math.MaxUint16 {
				return key, false
			}

			//nolint:gosec // bounded by math.MaxUint16 just above
			number = uint16(len(t.conditions) + 1)
			t.conditions[cond] = number
		}

		key[arg.Index] = number
	}

	return key, true
}

// resultOf returns the number of the result a rule yields, the same for
// every rule with the same result (see clause.sameResult). A syscall has no
// more distinct results than rules, which MaxArtifactRulesPerSyscall
// bounds, so the scan is short and the number fits.
func (t *conflictTracker) resultOf(next clause) uint16 {
	var number uint16

	for ; int(number) < len(t.results); number++ {
		if t.results[number].sameResult(next) {
			return number
		}
	}

	t.results = append(t.results, clause{action: next.action, errnoRet: next.errnoRet})

	return number
}

// The numbers operatorCode gives the operators, zero for an unknown one.
const (
	opUnknown uint8 = iota
	opNotEqual
	opLessThan
	opLessEqual
	opEqualTo
	opGreaterEqual
	opGreaterThan
	opMaskedEqual
)

// operatorCode numbers the known operators from one, and returns zero for
// any other.
func operatorCode(op specs.LinuxSeccompOperator) uint8 {
	switch op {
	case specs.OpNotEqual:
		return opNotEqual
	case specs.OpLessThan:
		return opLessThan
	case specs.OpLessEqual:
		return opLessEqual
	case specs.OpEqualTo:
		return opEqualTo
	case specs.OpGreaterEqual:
		return opGreaterEqual
	case specs.OpGreaterThan:
		return opGreaterThan
	case specs.OpMaskedEqual:
		return opMaskedEqual
	default:
		return opUnknown
	}
}

// record stores next and reports whether an earlier rule yields a different
// result while its filter is equal to, wider than, or narrower than the
// filter of next. A filter is wider when its conditions are a proper subset
// of the other's.
func (t *conflictTracker) record(next clause) bool {
	key, ok := t.keyOf(next.args)
	if !ok {
		// Unreachable for a profile that passes Validate, which is the only
		// one the conflict checks run on.
		return false
	}

	result := t.resultOf(next)
	conflict := slices.ContainsFunc(t.rules, func(earlier trackedRule) bool {
		return earlier.result != result && earlier.key.nested(key)
	})

	t.rules = append(t.rules, trackedRule{key: key, result: result})

	return conflict
}

// truncatedClause returns the rule libseccomp adds for a 32-bit
// architecture, which compares only the lower 32 bits of each value and
// therefore drops a masked comparison whose mask is empty there.
func truncatedClause(current clause) clause {
	args := make([]specs.LinuxSeccompArg, 0, len(current.args))

	for _, arg := range current.args {
		arg.Value &= lower32
		arg.ValueTwo &= lower32

		if !tautology(arg) {
			args = append(args, arg)
		}
	}

	current.args = args

	return current
}
