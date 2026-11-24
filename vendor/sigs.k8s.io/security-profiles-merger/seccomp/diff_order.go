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

// orderedGroups adds to groups, and returns, the groups of siblings whose
// rules may decide a call by the order a runtime adds them in, per syscall
// read exactly (see orderSensitiveGroups). groups may be nil.
func orderedGroups(
	rules map[string]*syscallRules, groups map[string]map[siblingKey]bool,
) map[string]map[siblingKey]bool {
	for name, current := range rules {
		if current.summary != nil || current.unconditional != nil {
			continue
		}

		for key := range orderSensitiveGroups(current.conditional) {
			if groups == nil {
				groups = make(map[string]map[siblingKey]bool)
			}

			if groups[name] == nil {
				groups[name] = make(map[siblingKey]bool)
			}

			groups[name][key] = true
		}
	}

	return groups
}

// orderSensitiveGroups returns the groups of siblings among the
// conditional rules of a syscall whose rules may decide a call by the order
// libseccomp adds them in. libseccomp compiles
// each condition to a node per 32-bit half it compares and orders sibling
// nodes by argument, operator and value whatever the add order, but keeps
// two siblings in the order they were added where they compare the same
// argument and half against the same value with different nodes: a
// different operator within one class (SCMP_CMP_LT and SCMP_CMP_LE, or
// SCMP_CMP_GT and SCMP_CMP_GE), or a different mask (SCMP_CMP_MASKED_EQ
// next to an equality, or two masks). Under default SCMP_ACT_ERRNO, KILL if
// a0 < 5 and LOG if a0 <= 5 decide read(3) by whichever comes first, and so
// do LOG if (a0 & 0xf0) == 0x10 and KILL if a0 == 0x11 for read(0x11),
// whose upper halves are compared with different masks. Checked against
// libseccomp 2.6.1 with seccomp_export_pfc, which also shows conditions in
// one class against different values, such as a0 < 5 and a0 <= 7, and in
// different classes, such as a0 < 5 and a0 > 5, compiled alike in either
// order.
//
// Only rules with different results and outside the safe shapes can tell
// two orders apart, so this returns, for a syscall outside them, the groups
// of siblings holding two nodes and two results: with both, some pair of
// their conditions differs in both. The range comparisons count with their
// lower half only, since libseccomp tests the upper half of each of them
// with the same nodes; an SCMP_CMP_NE counts as the SCMP_CMP_EQ it
// compiles to. Only the order within such a group matters, so Diff
// compares the order of the rules of each group rather than of every rule
// of the syscall.
func orderSensitiveGroups(clauses []clause) map[siblingKey]bool {
	if safeShape(clauses) {
		return nil
	}

	groups := make(map[siblingKey]*siblingGroup)

	var sensitive map[siblingKey]bool

	for _, current := range clauses {
		for _, arg := range current.args {
			nodes, count := siblingNodes(arg)

			for _, node := range nodes[:count] {
				if !recordSibling(groups, node, current) {
					continue
				}

				if sensitive == nil {
					sensitive = make(map[siblingKey]bool)
				}

				sensitive[node.key] = true
			}
		}
	}

	return sensitive
}

// recordSibling files a node of a rule in its group and reports whether the
// group then holds two nodes and two results.
func recordSibling(groups map[siblingKey]*siblingGroup, node keyedSibling, rule clause) bool {
	group, ok := groups[node.key]
	if !ok {
		groups[node.key] = &siblingGroup{node: node.node, result: rule}

		return false
	}

	group.nodes = group.nodes || group.node != node.node
	group.results = group.results || !group.result.sameResult(rule)

	return group.nodes && group.results
}

// siblingKey is what makes two nodes libseccomp compiles siblings whose
// order it takes from the rules: the argument, the half compared, the
// operator class, and the value compared against.
type siblingKey struct {
	index uint
	upper bool
	class siblingClass
	datum uint32
}

// siblingClass is an operator class libseccomp orders apart from the
// others.
type siblingClass uint8

const (
	// classEquality holds SCMP_CMP_EQ, SCMP_CMP_NE and SCMP_CMP_MASKED_EQ.
	classEquality siblingClass = iota
	// classBelow holds SCMP_CMP_LT and SCMP_CMP_LE.
	classBelow
	// classAbove holds SCMP_CMP_GT and SCMP_CMP_GE.
	classAbove
)

// siblingNode is what tells two siblings apart: the operator a node
// compares with and its mask.
type siblingNode struct {
	op   specs.LinuxSeccompOperator
	mask uint32
}

// siblingGroup records the nodes and results of the conditions sharing a
// siblingKey: the first node and result, and whether another one differs.
type siblingGroup struct {
	node           siblingNode
	result         clause
	nodes, results bool
}

// keyedSibling is one node a condition compiles to.
type keyedSibling struct {
	key  siblingKey
	node siblingNode
}

// halfOf returns the upper or the lower 32 bits of a value.
func halfOf(value uint64, upper bool) uint32 {
	if upper {
		value >>= 32
	}

	return uint32(value & math.MaxUint32)
}

// siblingNodes returns the nodes libseccomp compiles a condition to, as far
// as their order goes (see orderSensitiveGroups), and how many there are.
func siblingNodes(arg specs.LinuxSeccompArg) ([2]keyedSibling, int) {
	var nodes [2]keyedSibling

	switch arg.Op {
	case specs.OpLessThan, specs.OpGreaterEqual, specs.OpLessEqual, specs.OpGreaterThan:
		class := classAbove
		if arg.Op == specs.OpLessThan || arg.Op == specs.OpLessEqual {
			class = classBelow
		}

		nodes[0] = keyedSibling{
			key: siblingKey{
				index: arg.Index, upper: false, class: class, datum: halfOf(arg.Value, false),
			},
			node: siblingNode{op: arg.Op, mask: math.MaxUint32},
		}

		return nodes, 1
	case specs.OpEqualTo, specs.OpNotEqual, specs.OpMaskedEqual:
		operator, mask, datum := specs.OpEqualTo, uint64(math.MaxUint64), arg.Value
		if arg.Op == specs.OpMaskedEqual {
			operator, mask, datum = specs.OpMaskedEqual, arg.Value, arg.ValueTwo
		}

		for half, upper := range []bool{true, false} {
			nodes[half] = keyedSibling{
				key: siblingKey{
					index: arg.Index, upper: upper, class: classEquality,
					datum: halfOf(datum, upper),
				},
				node: siblingNode{op: operator, mask: halfOf(mask, upper)},
			}
		}

		return nodes, len(nodes)
	default:
		return nodes, 0
	}
}

// addOrder is the order a runtime adds the rules of the syscalls in some
// order-sensitive groups in (see orderSensitiveGroups), read from the rules
// collectRules keeps for them: every rule of each syscall, and for each
// group the nodes its rules fill, in the order each first appears.
type addOrder struct {
	rules  map[string][]clause
	groups map[string]map[siblingKey][]groupEntry
}

// groupEntry is one node of a group of siblings and the result of a rule
// that fills it. Rules that fill one node with one result add nothing to
// the order the first of them gives, so they count as one entry.
type groupEntry struct {
	node   siblingNode
	result string
}

// loadAddOrder reads the add order of the syscalls in groups from the rules
// collectRules read for one side, which keeps the conditional rules in the
// order a runtime adds them, without exact repeats, and none next to an
// unconditional rule, which hides them. It reads every syscall of groups
// from the side's own rules, so what a diff reports for one side does not
// depend on the other.
func loadAddOrder(rules map[string]*syscallRules, groups map[string]map[siblingKey]bool) addOrder {
	order := addOrder{
		rules:  make(map[string][]clause, len(groups)),
		groups: make(map[string]map[siblingKey][]groupEntry, len(groups)),
	}

	for name, keys := range groups {
		current := rules[name]
		if current == nil || current.summary != nil {
			continue
		}

		if current.unconditional != nil {
			order.rules[name] = []clause{*current.unconditional}

			continue
		}

		order.rules[name] = slices.Clone(current.conditional)
		order.groups[name] = groupOrder(current.conditional, keys)
	}

	return order
}

// groupOrder returns, for each of the groups in keys, the entries the rules
// fill in the order each first appears.
func groupOrder(clauses []clause, keys map[siblingKey]bool) map[siblingKey][]groupEntry {
	type filed struct {
		key   siblingKey
		entry groupEntry
	}

	entries := make(map[siblingKey][]groupEntry, len(keys))
	seen := make(map[filed]struct{})

	for _, current := range clauses {
		result := resultKey(current.action, current.errnoRet, nil)

		for _, arg := range current.args {
			nodes, count := siblingNodes(arg)

			for _, node := range nodes[:count] {
				entry := filed{key: node.key, entry: groupEntry{node: node.node, result: result}}
				if _, dup := seen[entry]; dup || !keys[node.key] {
					continue
				}

				seen[entry] = struct{}{}
				entries[node.key] = append(entries[node.key], entry.entry)
			}
		}
	}

	return entries
}

// sameGroups reports whether both sides add the rules of each of the groups
// of a syscall in the same order.
func (order addOrder) sameGroups(other addOrder, name string, groups map[siblingKey]bool) bool {
	for key := range groups {
		if !slices.Equal(order.groups[name][key], other.groups[name][key]) {
			return false
		}
	}

	return true
}

// entries returns the rules of a syscall in the order a runtime adds them,
// as a diff reports a syscall whose rules differ only in that order: sorted,
// the two sides would list the same rules.
func (order addOrder) entries(name string) []SyscallEntry {
	entries := make([]SyscallEntry, 0, len(order.rules[name]))

	for _, current := range order.rules[name] {
		entries = append(entries, syscallEntry(name, clauseToSyscall(name, current)))
	}

	return entries
}
