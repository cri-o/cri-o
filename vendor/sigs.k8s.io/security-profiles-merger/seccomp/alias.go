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
	"maps"
	"slices"
	"strings"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"sigs.k8s.io/security-profiles-merger/internal/merge"
)

// syscallAlias is a pair of syscall names to which libseccomp gives one
// pseudo number on an architecture that has neither as a syscall, the
// first name sorting before the second. Wherever both names resolve to that
// number, libseccomp reads the rules of either as rules of one syscall: it
// refuses with EEXIST, or keeps only one of, a rule of one that conflicts
// with a rule of the other, and evaluates the rest as one rule set.
type syscallAlias [2]string

// syscallAliases are the aliases libseccomp knows. libseccomp 2.5.2
// through 2.6.2 define __PNR_switch_endian and __PNR_sys_debug_setcontext
// as -10191, and only the PowerPC architectures have the two as syscalls.
// The libseccomp tests of this package check that no other two names share
// a pseudo number.
var syscallAliases = []syscallAlias{
	{"switch_endian", "sys_debug_setcontext"},
}

// collapses reports whether libseccomp reads the rules of both names of
// the alias as rules of one syscall in a filter loaded from a profile
// listing archs on a node of the native architecture, or on a node of any
// architecture a runtime can be native on where native is empty (see
// possibleNatives). That holds in two cases:
//
//   - a covered architecture lacks both names and keeps their rules, under
//     the one pseudo number;
//   - the native architecture lacks both names, so a runtime resolves both
//     to the pseudo number, and a covered architecture places a rule on
//     either: libseccomp translates a rule for another architecture from
//     the number by name, so the rules of both land on the same syscall
//     there, even where that architecture has both. On x86_64, a rule on
//     switch_endian lands on sys_debug_setcontext of ppc64le.
//
// An architecture that multiplexes the socket and SysV IPC syscalls drops
// the rules of a syscall it lacks, so a filter covering only such
// architectures loads no rule on either name.
func (alias syscallAlias) collapses(archs []specs.Arch, native specs.Arch) bool {
	if native == "" {
		return slices.ContainsFunc(possibleNatives, func(arch specs.Arch) bool {
			return alias.collapses(archs, arch)
		})
	}

	first, second := syscallPlacements[alias[0]], syscallPlacements[alias[1]]

	// The architectures on which a covered block reads the two as one.
	joined := first.kept & second.kept
	if !alias.nativeHas(native) {
		joined |= first.direct | first.kept | second.direct | second.kept
	}

	return joined&archMask(coveredArchitectures(archs, native)) != 0
}

// nativeHas reports whether every release of libseccomp places a rule on
// one of the names of the alias under its own number on the native
// architecture, so that a runtime there resolves the names to different
// numbers and libseccomp translates their rules to every other
// architecture apart.
func (alias syscallAlias) nativeHas(native specs.Arch) bool {
	bit := archBit(native)

	return (syscallPlacements[alias[0]].direct|syscallPlacements[alias[1]].direct)&bit != 0
}

// collapsedAliases returns the aliases whose names libseccomp reads as one
// syscall in a filter loaded from a profile listing archs on a node of the
// native architecture (see syscallAlias.collapses), or nil.
func collapsedAliases(archs []specs.Arch, native specs.Arch) []syscallAlias {
	var collapsed []syscallAlias

	for _, alias := range syscallAliases {
		if alias.collapses(archs, native) {
			collapsed = append(collapsed, alias)
		}
	}

	return collapsed
}

// aliasesByName maps both names of every alias given to the alias, or
// returns nil for none.
func aliasesByName(aliases []syscallAlias) map[string]syscallAlias {
	if len(aliases) == 0 {
		return nil
	}

	byName := make(map[string]syscallAlias)

	for _, alias := range aliases {
		byName[alias[0]] = alias
		byName[alias[1]] = alias
	}

	return byName
}

// dropAliasRepeats returns the entries with every name of an alias given
// left out of an entry that names the alias before it by its other name.
// libseccomp reads the rules such an entry adds for the two names as the
// same rules of one syscall, so the entry adds them once. A name an entry
// repeats is kept, as the limits count it for any syscall. It returns the
// entries themselves where none names both names of an alias, and a copy
// otherwise.
func dropAliasRepeats(syscalls []specs.LinuxSyscall, aliases []syscallAlias) []specs.LinuxSyscall {
	byName := aliasesByName(aliases)
	if byName == nil {
		return syscalls
	}

	kept := func(names []string) []string {
		// The name each alias was first named by in the entry.
		named := make(map[syscallAlias]string)

		return slices.DeleteFunc(slices.Clone(names), func(name string) bool {
			alias, ok := byName[name]
			if !ok {
				return false
			}

			if first, seen := named[alias]; seen {
				return first != name
			}

			named[alias] = name

			return false
		})
	}

	repeats := func(entry specs.LinuxSyscall) bool {
		return len(kept(entry.Names)) != len(entry.Names)
	}

	if !slices.ContainsFunc(syscalls, repeats) {
		return syscalls
	}

	result := slices.Clone(syscalls)
	for idx := range result {
		result[idx].Names = kept(result[idx].Names)
	}

	return result
}

// countLabels names the syscalls of counts joined by joinAliasCounts in
// messages.
type countLabels map[string]string

// of returns the label of a syscall: both names of an alias counted as
// one, or the name itself.
func (labels countLabels) of(name string) string {
	if label, ok := labels[name]; ok {
		return label
	}

	return merge.QuoteBounded(name)
}

// joinAliasCounts returns counts by syscall name with the counts of the two
// names of every alias given added up under the first name, where either
// holds one. It copies counts where it joins any.
func joinAliasCounts[T int | uint64](counts map[string]T, aliases []syscallAlias) map[string]T {
	cloned := false

	for _, alias := range aliases {
		first, hasFirst := counts[alias[0]]
		second, hasSecond := counts[alias[1]]

		if !hasSecond {
			continue
		}

		if !cloned {
			counts = maps.Clone(counts)
			cloned = true
		}

		if hasFirst {
			second += first
		}

		counts[alias[0]] = second
		delete(counts, alias[1])
	}

	return counts
}

// aliasLabels returns the labels of the counts joinAliasCounts joins under
// the first name of each alias given: the names of the alias the entries
// use, both where they use both.
func aliasLabels(syscalls []specs.LinuxSyscall, aliases []syscallAlias) countLabels {
	labels := make(countLabels)

	for _, alias := range aliases {
		used := alias.usedNames(syscalls)

		quoted := make([]string, 0, len(used))
		for _, name := range used {
			quoted = append(quoted, merge.QuoteBounded(name))
		}

		if len(quoted) > 0 {
			labels[alias[0]] = strings.Join(quoted, " and ")
		}
	}

	return labels
}

// usedNames returns the names of the alias some entry of the lists names,
// in the order of the alias.
func (alias syscallAlias) usedNames(lists ...[]specs.LinuxSyscall) []string {
	return slices.DeleteFunc(slices.Clone(alias[:]), func(name string) bool {
		return !slices.ContainsFunc(lists, func(syscalls []specs.LinuxSyscall) bool {
			return slices.ContainsFunc(syscalls, func(entry specs.LinuxSyscall) bool {
				return slices.Contains(entry.Names, name)
			})
		})
	})
}

// joinedAliases returns the aliases whose names a runtime on a node of the
// native architecture resolves to one number, as the native architecture
// lacks both, or nil where native is empty. libseccomp then reads their
// rules as rules of one syscall in every block that places them, so
// reading the two names as one syscall is exact there (see joinAliases).
func joinedAliases(native specs.Arch) []syscallAlias {
	if native == "" {
		return nil
	}

	var joined []syscallAlias

	for _, alias := range syscallAliases {
		if !alias.nativeHas(native) {
			joined = append(joined, alias)
		}
	}

	return joined
}

// joinAliases returns the entries with the second name of every alias
// given spelled as the first, so that the entries read as libseccomp reads
// them where it reads both names as one syscall. It returns the entries
// themselves where none names such an alias, and a copy otherwise.
func joinAliases(syscalls []specs.LinuxSyscall, aliases []syscallAlias) []specs.LinuxSyscall {
	if len(aliases) == 0 {
		return syscalls
	}

	joined := make(map[string]string)

	for _, alias := range aliases {
		joined[alias[1]] = alias[0]
	}

	renamed := func(entry specs.LinuxSyscall) bool {
		return slices.ContainsFunc(entry.Names, func(name string) bool {
			_, ok := joined[name]

			return ok
		})
	}

	if !slices.ContainsFunc(syscalls, renamed) {
		return syscalls
	}

	result := slices.Clone(syscalls)

	for idx := range result {
		if !renamed(result[idx]) {
			continue
		}

		names := slices.Clone(result[idx].Names)
		for nameIdx, name := range names {
			if first, ok := joined[name]; ok {
				names[nameIdx] = first
			}
		}

		result[idx].Names = names
	}

	return result
}

// unifiedAliases returns the aliases whose names libseccomp may read as one
// syscall in a filter loaded from a profile listing archs on a node of the
// native architecture (see syscallAlias.collapses), less those the native
// architecture lacks, which a merge joins (see joinedAliases): the merges
// give both names of these the same rules instead where their rules
// conflict (see unifyResultAliases).
func unifiedAliases(archs []specs.Arch, native specs.Arch) []syscallAlias {
	return slices.DeleteFunc(collapsedAliases(archs, native), func(alias syscallAlias) bool {
		return native != "" && !alias.nativeHas(native)
	})
}

// shareJoinedEntries returns the merged entries with every entry on the
// first name of an alias the merges join (see joinedAliases) repeated on
// the second, so that a result names both, as an input may name either,
// with the same rules, which libseccomp loads as the rules of one syscall
// there. Every entry names one syscall.
func (m ruleMerger) shareJoinedEntries(entries []specs.LinuxSyscall) []specs.LinuxSyscall {
	for _, alias := range joinedAliases(m.native) {
		for idx := range entries {
			if entries[idx].Names[0] != alias[0] {
				continue
			}

			shared := entries[idx]
			shared.Names = []string{alias[1]}
			shared.ErrnoRet = merge.ClonePtr(shared.ErrnoRet)
			shared.Args = slices.Clone(shared.Args)
			entries = append(entries, shared)
		}
	}

	return entries
}

// unifyResultAliases gives both names of an alias the same rules in a merge
// result where the rules the merge gave each name conflict when read as the
// rules of one syscall (see conflictAsOne), and the filter the result is
// loaded into may read them so, which the merge cannot tell: on a PowerPC
// node, which has both, in a filter covering an architecture that keeps
// both under one number, and on a node not known (see unifiedAliases). The
// rules are what the two merge to in the direction of m, as if from two
// inputs with the default of the result, which finishRules settles into a
// safe shape as it settles any merge. Every entry of the result names one
// syscall.
//
// That keeps the merge sound and the result loadable. A PowerPC block
// reads the names as two syscalls on a PowerPC node, where the rules of
// each name, a safe shape libseccomp evaluates as the merge model does,
// are as restrictive (intersection) or as permissive (union) as the merge
// gave that name, or more. Every other block that reads them as one is
// either of an architecture that lacks both, where their rules apply to no
// call, or a PowerPC block on a node of another architecture, whose kernel
// never enters it; libseccomp only has to load the rules there, and it
// loads the same rules on both names as one rule set. A result whose rules
// on the two do not conflict loads there as they are, and is left alone.
func (m ruleMerger) unifyResultAliases(result *specs.LinuxSeccomp) {
	aliases := unifiedAliases(result.Architectures, m.native)
	if len(aliases) == 0 {
		return
	}

	def := defaultClause(result)
	rules := collectRules(result.Syscalls, def)

	for _, alias := range aliases {
		first, second := rules[alias[0]], rules[alias[1]]
		if first == nil || second == nil || !conflictAsOne(first, second) {
			continue
		}

		fallback, conditional := m.mergeRules(first, second, def, def)
		fallback, conditional = m.finishRules(def, fallback, conditional)

		result.Syscalls = slices.DeleteFunc(result.Syscalls, func(entry specs.LinuxSyscall) bool {
			return entry.Names[0] == alias[0] || entry.Names[0] == alias[1]
		})

		for _, name := range alias {
			result.Syscalls = appendRules(result.Syscalls, name, def, fallback, conditional)
		}
	}
}

// conflictAsOne reports whether the rules of the two names of an alias in a
// merge result, read as the rules of one syscall, hold rules with
// different results that libseccomp refuses or evaluates in its own order,
// as ValidateArtifact checks them (see validateSyscallRules), with every
// value also read as on a 32-bit architecture.
func conflictAsOne(first, second *syscallRules) bool {
	var clauses []clause

	for _, current := range []*syscallRules{first, second} {
		if current.unconditional != nil {
			clauses = append(clauses, *current.unconditional)
		}

		clauses = append(clauses, current.conditional...)
	}

	trackers := [2]*conflictTracker{newConflictTracker(), newConflictTracker()}
	mixed := false

	var conditional []clause

	for _, next := range clauses {
		if trackers[0].record(next) || trackers[1].record(truncatedClause(next)) {
			return true
		}

		mixed = mixed || !next.sameResult(clauses[0])

		if !next.unconditional() {
			conditional = append(conditional, next)
		}
	}

	return mixed && !safeShape(dedupeClauses(conditional))
}
