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
	"maps"
	"slices"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"sigs.k8s.io/security-profiles-merger/internal/merge"
)

// clause is a single rule for one syscall as a runtime adds it to
// libseccomp: an action, an optional errno, and optional argument filters.
// A clause without args is unconditional. See the package documentation
// for how libseccomp evaluates the clauses of a syscall.
//
// The merge computes over a working model of a syscall's clauses: an
// unconditional clause decides every call; otherwise the least restrictive
// clause matching a call decides it, and the fallback (the profile default)
// decides calls no clause matches. This model agrees with libseccomp on the
// safe shapes, where at most one result applies to any call, and the merge
// only reads and writes safe shapes, so its results hold for what runtimes
// load.
type clause struct {
	action   specs.LinuxSeccompAction
	errnoRet *uint
	args     []specs.LinuxSeccompArg
}

func (c clause) unconditional() bool { return len(c.args) == 0 }

// sameResult reports whether two clauses yield the same runtime effect,
// ignoring their argument filters. ErrnoRet only matters for actions that
// return it to the caller; clauses carry it in runtime form, so an unset
// value on such an action has already become EPERM.
func (c clause) sameResult(other clause) bool {
	if !actionsEquivalent(c.action, other.action) {
		return false
	}

	return !errnoSignificant(c.action) || compareUintPtr(c.errnoRet, other.errnoRet) == 0
}

// pickClause selects between two clauses using the given action preference,
// which returns one of the two actions it is given. On a tie the left clause
// wins, so ErrnoRet comes from the leftmost profile: whichever of two
// equivalent actions pick returns is equivalent to the left one.
func pickClause(
	left, right clause,
	pick func(first, second specs.LinuxSeccompAction) specs.LinuxSeccompAction,
) clause {
	if actionsEquivalent(pick(left.action, right.action), left.action) {
		return left
	}

	return right
}

// syscallRules holds every clause of one profile for a single syscall name.
// Past the collectRules budget it holds a summary of them instead, which
// settleInputs turns into the one clause a collapse picks before any merge
// reads the fallback or the conditional clauses.
type syscallRules struct {
	unconditional *clause
	conditional   []clause
	summary       *clauseSummary
	// narrowFirst is the first clause, in the order a runtime adds them,
	// that holds for every call where libseccomp compares 32 bits: an
	// unconditional one, or one whose every condition is a
	// SCMP_CMP_MASKED_EQ with a mask that sets only upper bits, which
	// libseccomp drops there (see truncatedClause). Any other condition
	// keeps a lower half to test: SCMP_CMP_EQ against 1<<32 tests for 0.
	// libseccomp keeps the first unconditional rule of a syscall, so on a
	// 32-bit architecture this decides every call where unconditional
	// decides them elsewhere.
	narrowFirst *clause
}

// forEachClause expands syscall entries into clauses the way a runtime loads
// them and calls visit once per entry index, syscall name, and clause.
// Multi-name entries contribute one clause per name, and entries equal to
// def are skipped, as runtimes skip them. Names for which
// skip reports true are not visited at all, so a caller that has already
// settled a syscall pays nothing for its remaining names. A nil skip visits
// every name.
func forEachClause(
	syscalls []specs.LinuxSyscall, def *clause,
	skip func(name string) bool,
	visit func(idx int, name string, next clause),
) {
	for idx := range syscalls {
		entry := &syscalls[idx]

		clauses := slices.DeleteFunc(entryClauses(entry), func(next clause) bool {
			return next.sameResult(*def)
		})

		for _, name := range entry.Names {
			if skip != nil && skip(name) {
				continue
			}

			for _, next := range clauses {
				visit(idx, name, next)
			}
		}
	}
}

// maxLoadedClauses bounds how many clauses collectRules materializes for one
// input: the read budget of the Cost bounds section of the package
// documentation, which says why the clauses can outgrow the profile. Past
// this bound an input is read in summarized form instead, which costs one
// pass over the entries and no memory per clause (see summarizeRules).
//
// No profile a runtime should accept comes near the bound. ValidateArtifact
// rejects an artifact loading more than MaxArtifactRules rules, which is a
// quarter of it, so an artifact is always read exactly.
const maxLoadedClauses = 1 << 16

// collectRules splits syscall entries into per-name clause sets as a runtime
// loads them. Entries equal to the profile default are skipped when def is
// non-nil, the first unconditional entry wins over later ones and hides the
// conditional entries of its name, and exact duplicates are dropped.
//
// An input whose entries would load more than maxLoadedClauses rules in
// total has as many of its syscalls summarized as it takes to fit, largest
// first: such a syscall is read as the clause a collapse picks, which is at
// least as restrictive as its rules for intersection and at least as
// permissive for union. The syscalls that fit are read exactly, so one
// oversized syscall costs its own filters rather than every filter of the
// profile.
func collectRules(syscalls []specs.LinuxSyscall, def *clause) map[string]*syscallRules {
	summarized := summarizedNames(syscalls, def)

	rules := summarizeRules(syscalls, def, summarized)

	skip := func(name string) bool { return summarized[name] }
	if len(summarized) == 0 {
		skip = nil
	}

	forEachClause(syscalls, def, skip, func(_ int, name string, next clause) {
		current, ok := rules[name]
		if !ok {
			current = &syscallRules{}
			rules[name] = current
		}

		current.add(next)
	})

	for _, current := range rules {
		if current.unconditional != nil {
			current.conditional = nil

			continue
		}

		current.conditional = dedupeClauses(current.conditional)
	}

	return rules
}

// summarizedNames returns the syscalls collectRules reads in summarized
// form, or nil when every syscall fits the budget.
func summarizedNames(syscalls []specs.LinuxSyscall, def *clause) map[string]bool {
	total := totalClauses(syscalls, def)
	if total <= maxLoadedClauses {
		return nil
	}

	return oversizedNames(clauseCounts(syscalls, def), total)
}

// entryClauseCount counts the clauses one entry loads for each of its names,
// the way runtimes add them but without expanding the entry: none when the
// entry equals the profile default, one per condition when it repeats an
// argument index, and one otherwise.
func entryClauseCount(entry *specs.LinuxSyscall, def *clause) uint64 {
	if baseClause(entry).sameResult(*def) {
		return 0
	}

	if hasRepeatedIndex(entry.Args) {
		return uint64(len(entry.Args))
	}

	return 1
}

// loadsRule reports whether an entry loads any rule: whether it names a
// syscall and differs from the profile default, which runtimes skip an
// entry for.
func loadsRule(entry *specs.LinuxSyscall, def *clause) bool {
	return len(entry.Names) > 0 && entryClauseCount(entry, def) > 0
}

// totalClauses returns how many clauses the entries load in total, so that a
// caller can bound the expansion before paying for it.
//
// The count is a uint64 because it is the product of two counts an untrusted
// profile chooses, which overflows an int on a 32-bit platform.
func totalClauses(syscalls []specs.LinuxSyscall, def *clause) uint64 {
	var total uint64

	for idx := range syscalls {
		total += entryClauseCount(&syscalls[idx], def) * uint64(len(syscalls[idx].Names))
	}

	return total
}

// oversizedNames returns the syscalls to read in summarized form so that the
// clauses the others load stay inside maxLoadedClauses. The largest go
// first, and only as many as the bound requires, so a profile spends its
// budget on the syscalls that fit rather than on the one that does not.
// Names of equal size are ordered by name, so which syscall loses its
// filters never depends on the order the entries arrive in.
func oversizedNames(counts map[string]uint64, total uint64) map[string]bool {
	names := slices.Collect(maps.Keys(counts))
	slices.SortFunc(names, func(first, second string) int {
		if counts[first] != counts[second] {
			return cmp.Compare(counts[second], counts[first])
		}

		return cmp.Compare(first, second)
	})

	summarized := make(map[string]bool)

	for _, name := range names {
		if total <= maxLoadedClauses {
			break
		}

		summarized[name] = true
		total -= counts[name]
	}

	return summarized
}

// clauseCounts returns how many clauses each syscall name loads, counted as
// totalClauses counts them, whose total they add up to.
func clauseCounts(syscalls []specs.LinuxSyscall, def *clause) map[string]uint64 {
	counts := make(map[string]uint64)

	for idx := range syscalls {
		entry := &syscalls[idx]

		rules := entryClauseCount(entry, def)
		if rules == 0 {
			continue
		}

		for _, name := range entry.Names {
			counts[name] += rules
		}
	}

	return counts
}

// clauseSummary is how an over-budget input is read: the two clauses a
// collapse can pick for one syscall. Folding every clause into the most and
// the least restrictive one costs no memory per clause, and folds them with
// the preference and tie-break collapseAll uses, so collapsing a summary
// yields the clause collapsing the whole set would have yielded.
type clauseSummary struct {
	strictest clause
	loosest   clause
}

// fold records one clause. Summaries stand for a collapse, which decides
// every call of its syscall, so the argument filters are dropped here.
func (s *clauseSummary) fold(next clause) *clauseSummary {
	next.args = nil

	if s == nil {
		next.errnoRet = merge.ClonePtr(next.errnoRet)

		return &clauseSummary{strictest: next, loosest: next}
	}

	s.strictest = pickClause(s.strictest, next, moreRestrictive)
	s.loosest = pickClause(s.loosest, next, lessRestrictive)

	return s
}

func (s *clauseSummary) foldSummary(other *clauseSummary) *clauseSummary {
	return s.fold(other.strictest).fold(other.loosest)
}

// pick returns the clause a collapse in the given direction takes.
func (s *clauseSummary) pick(intersect bool) clause {
	if intersect {
		return s.strictest
	}

	return s.loosest
}

// summarizeRules reads entries into per-name summaries, which is what
// collectRules falls back to past maxLoadedClauses. Every name of an entry
// loads the same clauses, so each entry is folded once and its fold applied
// per name, which keeps the pass linear in the size of the input instead of
// in the clauses it would expand to. Only the syscalls in summarized are
// read; the rest are left to the caller, which reads them exactly.
func summarizeRules(
	syscalls []specs.LinuxSyscall, def *clause, summarized map[string]bool,
) map[string]*syscallRules {
	rules := make(map[string]*syscallRules)

	if len(summarized) == 0 {
		return rules
	}

	for idx := range syscalls {
		entry := &syscalls[idx]

		var entrySummary *clauseSummary

		for _, next := range entryClauses(entry) {
			if next.sameResult(*def) {
				continue
			}

			entrySummary = entrySummary.fold(next)
		}

		if entrySummary == nil {
			continue
		}

		for _, name := range entry.Names {
			if !summarized[name] {
				continue
			}

			current, ok := rules[name]
			if !ok {
				current = &syscallRules{}
				rules[name] = current
			}

			current.summary = current.summary.foldSummary(entrySummary)
		}
	}

	return rules
}

// entryClauses expands one syscall entry into clauses. An entry whose args
// repeat an argument index yields one single-condition clause per arg, as
// runc loads such entries; every other entry yields exactly one clause.
// Conditions that hold for every value are dropped, as libseccomp drops them,
// so a clause left without conditions is unconditional.
func entryClauses(entry *specs.LinuxSyscall) []clause {
	base := baseClause(entry)

	if !hasRepeatedIndex(entry.Args) {
		base.args = loadedArgs(entry.Args)

		return []clause{base}
	}

	clauses := make([]clause, 0, len(entry.Args))

	for _, arg := range entry.Args {
		next := base
		next.errnoRet = merge.ClonePtr(base.errnoRet)
		next.args = loadedArgs([]specs.LinuxSeccompArg{arg})
		clauses = append(clauses, next)
	}

	return clauses
}

// baseClause returns the result an entry applies, as a clause without
// conditions: its action and errno as a runtime loads them.
func baseClause(entry *specs.LinuxSyscall) clause {
	return clause{
		action:   canonicalAction(entry.Action),
		errnoRet: runtimeErrno(entry.Action, entry.ErrnoRet),
	}
}

// loadedArgs returns the sorted, canonical conditions libseccomp keeps from a
// rule, or nil when it keeps none.
func loadedArgs(args []specs.LinuxSeccompArg) []specs.LinuxSeccompArg {
	loaded := slices.DeleteFunc(sortedArgs(args), tautology)
	if len(loaded) == 0 {
		return nil
	}

	return loaded
}

// add records a clause. The first unconditional clause wins; conditional
// clauses accumulate.
func (r *syscallRules) add(next clause) {
	next.errnoRet = merge.ClonePtr(next.errnoRet)

	if r.narrowFirst == nil && truncatedClause(next).unconditional() {
		first := next
		first.args = nil
		r.narrowFirst = &first
	}

	if !next.unconditional() {
		r.conditional = append(r.conditional, next)

		return
	}

	if r.unconditional == nil {
		r.unconditional = &next
	}
}

// fallback returns the clause applied when no conditional clause matches:
// the unconditional clause if present, otherwise the given profile default.
func (r *syscallRules) fallback(def *clause) *clause {
	if r != nil && r.unconditional != nil {
		return r.unconditional
	}

	return def
}

func (r *syscallRules) conditionals() []clause {
	if r == nil {
		return nil
	}

	return r.conditional
}

// maxPairwiseClauses bounds the product of the conditional clause counts of
// the two sides of a merge for one syscall. Both directions compare every
// conditional clause against every clause of the other side, and
// intersection emits a clause per overlapping pair, so this product is the
// work and the output size a merge takes on for one syscall. Past it the
// syscall collapses to one unconditional clause on the safe side of the
// merge direction (see collapse), which is what the merge does anyway
// wherever the exact result is not expressible. It admits an artifact at
// MaxArtifactRulesPerSyscall against a baseline with a dozen filtered
// entries for the same syscall.
const maxPairwiseClauses = 4096

// maxSyscallClauses bounds the conditional clause count of a union for one
// syscall with a fallback. Union raises every clause that overlaps a
// redundant one (see raiseOverlapsOfRedundant), which compares the clauses
// pairwise, so past this bound the syscall collapses as for
// maxPairwiseClauses. Intersection does no such pass and has no bound on
// one side's clause count.
const maxSyscallClauses = 512

// maxPruneClauses bounds the clause count for which pruneDominated runs.
// Pruning may compare every clause against every other one and only
// removes clauses that can never decide a call, so skipping it past this bound
// leaves the result less minimal but unchanged in what it permits.
const maxPruneClauses = 512

// ruleMerger describes one merge direction over the clause model.
type ruleMerger struct {
	// intersect selects intersection semantics: a fallback exists only when
	// both sides have one, unconstrained regions are dropped, and a clause is
	// emitted for the overlap of two conditional clauses to express "both
	// filters hold". Union keeps every input clause instead.
	intersect bool
	// narrow reports that the filter the result is loaded into covers an
	// architecture libseccomp compiles 32-bit comparisons for, where a
	// condition against a value above 32 bits tests something else than it
	// says (see narrowArchitectures). settleInputs then reads a syscall
	// carrying such a condition the way it reads one outside the safe
	// shapes.
	narrow bool
	// narrowCovered reports that the filter the result is loaded into
	// covers an architecture libseccomp compiles 32-bit comparisons for,
	// whatever the conditions of the inputs, and the input being read
	// covers one too (see forInput). settleInputs then collapses a syscall
	// whose rule deciding every call there differs from the one deciding
	// every call elsewhere (see syscallRules.narrowFirst).
	narrowCovered bool
	// native is the architecture a runtime always adds to the filter of the
	// result, which the merge cannot drop from it: the one of the running
	// program.
	native specs.Arch
}

// intersectRulesFor and unionRulesFor return the merger of a node whose
// native architecture is the given one.
func intersectRulesFor(native specs.Arch) ruleMerger {
	return ruleMerger{intersect: true, native: native}
}

func unionRulesFor(native specs.Arch) ruleMerger {
	return ruleMerger{native: native}
}

// pick chooses the action for a call that both sides constrain: the more
// restrictive one for intersection, the less restrictive one for union.
func (m ruleMerger) pick(first, second specs.LinuxSeccompAction) specs.LinuxSeccompAction {
	if m.intersect {
		return moreRestrictive(first, second)
	}

	return lessRestrictive(first, second)
}

func (m ruleMerger) pickClause(left, right clause) clause {
	return pickClause(left, right, m.pick)
}

// collapse returns the clause that replaces conditional clauses whose effect
// the merge cannot rely on or cannot express: one unconditional clause
// combining def with every clause action, using the merge direction's
// preference. Whatever libseccomp does with the clauses, a call gets def or
// one of those actions, so the collapsed clause is at least as restrictive
// as any of them for intersection and at least as permissive for union.
//
// def is the profile default. It returns nil when the default decides every
// call instead, which is when the collapsed clause equals def.
func (m ruleMerger) collapse(def *clause, conditional []clause) *clause {
	collapsed := m.collapseAll(def, conditional)
	if collapsed.sameResult(*def) {
		return nil
	}

	return collapsed
}

// settleInputs reads the rules of one input the way the merge can rely on:
// a syscall whose conditional clauses do not form a safe shape, or that was
// summarized for exceeding the collectRules budget, is replaced by the clause
// collapse returns, or removed when that is nil. With m.narrow, so is a
// syscall with a condition against a value above 32 bits: libseccomp
// compares only the lower 32 bits of it on a 32-bit architecture, where the
// syscall's rules then mean something the model does not read, while a call
// still gets the default or the action of one of them.
//
// With m.narrowCovered, a syscall with an unconditional clause collapses
// too where a clause before it holds for every call on a 32-bit
// architecture and gives another result: libseccomp keeps that clause
// there and the unconditional one elsewhere, and one result cannot say
// both. The collapse picks between the two in the merge direction, so the
// syscall is read no more permissively (intersection) or no less
// (union) than either architecture loads it.
func (m ruleMerger) settleInputs(rules map[string]*syscallRules, def *clause) {
	for name, current := range rules {
		if current.summary != nil {
			current.unconditional = m.collapse(def, []clause{current.summary.pick(m.intersect)})
			current.summary = nil

			if current.unconditional == nil {
				delete(rules, name)
			}

			continue
		}

		if current.unconditional != nil {
			m.settleNarrowFirst(current)

			continue
		}

		var safe bool

		current.conditional, safe = unifyErrno(current.conditional)
		if safe && (!m.narrow || !slices.ContainsFunc(current.conditional, wideClause)) {
			continue
		}

		collapsed := m.collapse(def, current.conditional)
		if collapsed == nil {
			delete(rules, name)

			continue
		}

		current.unconditional = collapsed
		current.conditional = nil
	}
}

// forInput returns the merger that reads one input: a rule that holds for
// every call on a 32-bit architecture matters only where that input covers
// one, since on an architecture it does not cover its calls are killed
// whatever its rules say. Intersection results cover only what every input
// covers, so this narrows nothing there; a union covers what any input
// covers, and an input covering no 32-bit architecture is read as its
// 64-bit filter loads it.
func (m ruleMerger) forInput(input *specs.LinuxSeccomp) ruleMerger {
	m.narrowCovered = m.narrowCovered &&
		coversAny(input.Architectures, narrowArchitectures, m.native)

	return m
}

// settleNarrowFirst collapses the rules of a syscall with an unconditional
// clause where a 32-bit architecture the filter covers applies another
// clause to every call (see settleInputs). The default applies to no call
// of such a syscall on any architecture, so the collapse picks between the
// two clauses alone. The pick is one of the two, and collectRules drops
// every clause equal to the default, so it never equals the default and
// the syscall keeps a rule.
func (m ruleMerger) settleNarrowFirst(current *syscallRules) {
	first := current.narrowFirst
	if !m.narrowCovered || first == nil || first.sameResult(*current.unconditional) {
		return
	}

	picked := m.pickClause(*current.unconditional, *first)
	current.unconditional = &picked
}

// settleOutput makes the merged rules of one syscall safe to emit: they
// already are when they form a safe shape, and otherwise the syscall
// collapses. The fallback is nil whenever conditional clauses remain, since
// resolveMixed never leaves both.
func (m ruleMerger) settleOutput(
	def, fallback *clause, conditional []clause,
) (*clause, []clause) {
	conditional, safe := unifyErrno(conditional)
	if safe {
		return fallback, conditional
	}

	return m.collapse(def, conditional), nil
}

// unifyErrno gives clauses that apply one action but report different errno
// values the errno of the first of them. Such clauses are different results
// to libseccomp, which evaluates them in an order of its own, so the shape
// check refuses them and the syscall collapses to a single unconditional
// rule: a baseline filtering one argument and an artifact filtering another,
// one of them spelling the EPERM the other leaves implicit, would deny every
// call of a syscall both inputs allow. Unifying the errno keeps the filters
// and costs only the value a denied call reports, which is the trade
// finishRules already makes when it keeps a clause rather than its errno.
//
// The clauses must share one action: an action decides whether a call runs
// at all, so no action may be rewritten here. ValidateArtifact reports the
// same clauses as conflicting rather than unifying them, since an artifact
// says what it means and the ambiguity is the author's to resolve.
//
// Only clauses testing one argument for equality are unified. That is the
// shape a baseline and an artifact filtering different arguments produce,
// which is the case worth keeping filters for; every other operator leaves
// the clauses as they are, so a rule set whose evaluation order this
// package reads conservatively keeps being read that way rather than being
// made to look uniform.
//
// A clause set that is already a safe shape is left alone. Shape (c) holds
// single equalities on one argument index with any results, so libseccomp
// evaluates it exactly as written: rewriting its errno values there would
// lose what the inputs said for nothing, since no collapse threatens the
// filters.
//
// It also reports whether the clauses it returns form a safe shape, which
// every caller needs next and which it has to find out anyway.
func unifyErrno(clauses []clause) ([]clause, bool) {
	if safeShape(clauses) {
		return clauses, true
	}

	if !errnoUnifiable(clauses) {
		return clauses, false
	}

	unified := slices.Clone(clauses)
	for idx := range unified {
		unified[idx].errnoRet = clauses[0].errnoRet
	}

	return unified, safeShape(unified)
}

// errnoUnifiable reports whether unifyErrno may rewrite clauses that do not
// form a safe shape: they are several, they apply one errno-carrying action,
// each tests one argument for equality, no two test the same argument for
// the same value, and they do not all report the same errno already.
func errnoUnifiable(clauses []clause) bool {
	if len(clauses) < 2 || !errnoSignificant(clauses[0].action) {
		return false
	}

	differs := false
	filters := make(map[string]struct{}, len(clauses))

	for _, current := range clauses {
		if !actionsEquivalent(current.action, clauses[0].action) || !singleEquality(current) {
			return false
		}

		// Two clauses testing the same argument for the same value are a
		// conflict rather than a pair to unify: libseccomp refuses the
		// second rule, so which result a call gets is not something this
		// package can read, and the syscall must keep collapsing.
		filter := sortedArgsKey(current.args)
		if _, repeated := filters[filter]; repeated {
			return false
		}

		filters[filter] = struct{}{}

		if compareUintPtr(current.errnoRet, clauses[0].errnoRet) != 0 {
			differs = true
		}
	}

	return differs
}

// singleEquality reports whether a clause tests one argument for equality,
// which is the only filter shape unifyErrno rewrites.
func singleEquality(current clause) bool {
	return len(current.args) == 1 && current.args[0].Op == specs.OpEqualTo
}

// mergeRules merges the clauses of one syscall from two sides.
//
// For intersection the result never permits more than either input, and for
// union it never permits less, under the working model documented on the
// clause type. Where the exact result is not expressible, intersection
// falls back to the more restrictive and union to the less restrictive
// surrounding action.
//
// leftDef and rightDef are the profile defaults. The returned fallback is
// the unconditional clause of the result. The conditional clauses are
// not yet collapsed: callers run finishRules with the effective fallback,
// which is the merged default where the fallback is elided.
func (m ruleMerger) mergeRules(
	left, right *syscallRules,
	leftDef, rightDef *clause,
) (*clause, []clause) {
	leftFallback := left.fallback(leftDef)
	rightFallback := right.fallback(rightDef)
	fallback := m.pickClause(*leftFallback, *rightFallback)

	leftConds := left.conditionals()
	rightConds := right.conditionals()

	if m.exceedsClauseBudget(leftConds, rightConds) {
		return m.collapseAll(&fallback, leftConds, rightConds), nil
	}

	var conditional []clause

	conditional = append(conditional, m.adjustClauses(leftConds, rightConds, rightFallback)...)
	conditional = append(conditional, m.adjustClauses(rightConds, leftConds, leftFallback)...)

	if m.intersect {
		leftKeys := clauseKeys(leftConds)
		rightKeys := clauseKeys(rightConds)

		for leftIdx, leftClause := range leftConds {
			for rightIdx, rightClause := range rightConds {
				args, ok := conjoinClauseArgs(
					leftClause.args, rightClause.args,
					leftKeys[leftIdx], rightKeys[rightIdx],
				)
				if !ok {
					continue
				}

				picked := m.pickClause(leftClause, rightClause)
				picked.args = args
				conditional = append(conditional, picked)
			}
		}
	}

	return &fallback, conditional
}

// exceedsClauseBudget reports whether merging the two clause sets would take
// more work than the budget allows: the pairwise comparison both directions
// run, and for union the pairwise raising pass over all clauses. For
// intersection only the product counts, so a large clause set against an
// unconditional or absent rule never exceeds the budget. For union the
// raising pass is quadratic in the combined clause count, so the combined
// count is bounded as well, even when one side has no conditional clauses.
//
// The counts are widened to uint64 first: they come from untrusted profiles,
// and their product overflows an int on a 32-bit platform, which would turn
// the budget off exactly for the inputs it exists for.
func (m ruleMerger) exceedsClauseBudget(left, right []clause) bool {
	if uint64(len(left))*uint64(len(right)) > maxPairwiseClauses {
		return true
	}

	return !m.intersect && uint64(len(left))+uint64(len(right)) > maxSyscallClauses
}

// collapseAll reduces a syscall to one unconditional clause combining the
// fallback with every conditional clause of the given sides, using the merge
// direction's preference. For intersection the result is at least as
// restrictive as the exact merge everywhere, since no input applies an
// action more restrictive than the one picked here; for union it is at
// least as permissive, for the same reason. The fallback is the syscall's
// unconditional clause or its profile's default, so it is never nil.
func (m ruleMerger) collapseAll(fallback *clause, sides ...[]clause) *clause {
	collapsed := *fallback
	collapsed.errnoRet = merge.ClonePtr(fallback.errnoRet)

	for _, side := range sides {
		for _, current := range side {
			collapsed = m.pickClause(collapsed, current)
		}
	}

	collapsed.args = nil

	return &collapsed
}

// resolveMixed makes a merged rule set expressible: an unconditional entry
// would override every conditional entry of the same syscall at load time,
// so a non-nil fallback must not be emitted next to conditional clauses.
// A single one-condition clause is rewritten as the clause plus its
// complement carrying the fallback, which is exact and a safe shape. The
// two match disjoint calls, so a clause with the result of def is left out:
// the calls it matches fall through to def anyway, and a runtime skips such
// an entry. Otherwise the whole syscall collapses to one unconditional
// clause combining the fallback and every conditional action with the merge
// direction's preference, which is conservative in the safe direction.
func (m ruleMerger) resolveMixed(
	def, fallback *clause, conditional []clause,
) (*clause, []clause) {
	if fallback == nil || len(conditional) == 0 {
		return fallback, conditional
	}

	conditional = m.foldIntoFallback(fallback, conditional)
	if len(conditional) == 0 {
		return fallback, nil
	}

	if len(conditional) == 1 && len(conditional[0].args) == 1 {
		if complement, ok := complementArg(conditional[0].args[0]); ok {
			rest := clause{
				action:   fallback.action,
				errnoRet: merge.ClonePtr(fallback.errnoRet),
				args:     []specs.LinuxSeccompArg{complement},
			}

			if conditional[0].sameResult(*def) {
				return nil, []clause{rest}
			}

			return nil, sortClauses([]clause{conditional[0], rest})
		}
	}

	return m.collapseAll(fallback, conditional), nil
}

// foldIntoFallback drops conditional clauses that differ from the fallback
// only by errno. The rewrite in resolveMixed cannot keep their errno, and
// folding them first lets a single remaining filter use the exact complement
// form instead of collapsing the whole syscall. For union, a stricter clause
// overlapping a folded clause is raised to the fallback first, because the
// folded clause no longer shields the overlap.
func (m ruleMerger) foldIntoFallback(fallback *clause, conditional []clause) []clause {
	byArgs, order := groupByArgs(conditional)

	if !m.intersect {
		raiseOverlapsOfRedundant(byArgs, func(current clause) bool {
			return actionsEquivalent(current.action, fallback.action)
		}, *fallback)
	}

	kept := make([]clause, 0, len(order))

	for _, key := range order {
		current := byArgs[key]
		if actionsEquivalent(current.action, fallback.action) {
			continue
		}

		kept = append(kept, current)
	}

	return kept
}

// groupByArgs merges clauses with identical argument filters into the least
// restrictive one, which is the one the working model applies where they
// match, and returns them by argument key together with the keys in order
// of first appearance.
func groupByArgs(clauses []clause) (map[string]clause, []string) {
	byArgs := make(map[string]clause, len(clauses))
	order := make([]string, 0, len(clauses))

	for _, current := range clauses {
		key := sortedArgsKey(current.args)

		if existing, ok := byArgs[key]; ok {
			byArgs[key] = pickClause(existing, current, lessRestrictive)

			continue
		}

		byArgs[key] = current
		order = append(order, key)
	}

	return byArgs, order
}

// complementArg returns the condition matching exactly the values the given
// condition does not match. Masked comparisons have no complement.
func complementArg(arg specs.LinuxSeccompArg) (specs.LinuxSeccompArg, bool) {
	complement, ok := complementOps[arg.Op]
	if !ok {
		return specs.LinuxSeccompArg{}, false
	}

	return specs.LinuxSeccompArg{
		Index:    arg.Index,
		Value:    arg.Value,
		ValueTwo: 0,
		Op:       complement,
	}, true
}

// complementOps maps each comparison operator to the one matching exactly
// the remaining values. Masked comparisons have no complement.
var complementOps = map[specs.LinuxSeccompOperator]specs.LinuxSeccompOperator{
	specs.OpEqualTo:      specs.OpNotEqual,
	specs.OpNotEqual:     specs.OpEqualTo,
	specs.OpLessThan:     specs.OpGreaterEqual,
	specs.OpGreaterEqual: specs.OpLessThan,
	specs.OpLessEqual:    specs.OpGreaterThan,
	specs.OpGreaterThan:  specs.OpLessEqual,
}

// keyedClause pairs a clause with its precomputed argument key so that
// sorting does not rebuild the key on every comparison.
type keyedClause struct {
	key    string
	clause clause
}

func sortClauses(clauses []clause) []clause {
	keyed := make([]keyedClause, len(clauses))
	for idx := range clauses {
		keyed[idx] = keyedClause{key: sortedArgsKey(clauses[idx].args), clause: clauses[idx]}
	}

	return sortKeyedClauses(clauses[:0], keyed)
}

// sortKeyedClauses sorts clauses whose keys are known by them and appends
// the clauses in that order to dst.
func sortKeyedClauses(dst []clause, keyed []keyedClause) []clause {
	slices.SortFunc(keyed, func(a, b keyedClause) int {
		return cmp.Compare(a.key, b.key)
	})

	for idx := range keyed {
		dst = append(dst, keyed[idx].clause)
	}

	return dst
}

// clauseKeys returns the argument key of every clause, computed once.
func clauseKeys(clauses []clause) []string {
	keys := make([]string, len(clauses))
	for idx := range clauses {
		keys[idx] = sortedArgsKey(clauses[idx].args)
	}

	return keys
}

// adjustClauses returns one clause per entry of clauses, with the action
// combined against what the other side may apply inside the clause's
// argument region. For intersection this lowers the action to what both
// sides allow; for union it raises it to what either side allows.
//
// The other side's fallback applies inside the region unless an other-side
// clause matches everything the clause matches (see argsCover), in which
// case the fallback can never be reached there. Intersection also
// lowers the action by every overlapping other-side clause, because the
// working model applies the least restrictive matching clause. Union does
// not need that: every other-side clause is emitted on its own and raises
// the result wherever it matches.
func (m ruleMerger) adjustClauses(
	clauses, others []clause, otherFallback *clause,
) []clause {
	result := make([]clause, 0, len(clauses))

	for _, current := range clauses {
		adjusted, subsumed := m.adjustAgainstOthers(current, others)

		if !subsumed {
			adjusted = m.pickClause(adjusted, *otherFallback)
		}

		adjusted.args = current.args
		result = append(result, adjusted)
	}

	return result
}

// adjustAgainstOthers combines current with every overlapping other-side
// clause (intersection only) and reports whether an other-side clause
// subsumes current.
func (m ruleMerger) adjustAgainstOthers(
	current clause, others []clause,
) (clause, bool) {
	adjusted := current
	subsumed := false

	for _, other := range others {
		if argsDisjoint(current.args, other.args) {
			continue
		}

		if argsCover(other.args, current.args) {
			subsumed = true
		}

		if m.intersect {
			adjusted = m.pickClause(adjusted, other)
		}
	}

	return adjusted, subsumed
}

// conjoinClauseArgs returns the filter matching the overlap of two
// conditional clauses, given their precomputed argument keys. Identical
// filters are kept as-is; otherwise the filters must not be provably
// disjoint and must be conjoinable.
func conjoinClauseArgs(
	left, right []specs.LinuxSeccompArg, leftKey, rightKey string,
) ([]specs.LinuxSeccompArg, bool) {
	if leftKey == rightKey {
		return slices.Clone(left), true
	}

	if argsDisjoint(left, right) {
		return nil, false
	}

	return conjoinArgs(left, right)
}

// collapseClauses merges clauses with identical argument filters (keeping the
// least restrictive, since they always match together) and drops clauses
// that yield the same result as the fallback. Runtimes skip such entries at
// load time, so they cannot shield a call from a stricter overlapping clause;
// for union the stricter clause is raised to the fallback instead.
func (m ruleMerger) collapseClauses(clauses []clause, fallback *clause) []clause {
	byArgs, order := groupByArgs(clauses)

	if !m.intersect {
		raiseOverlapsOfRedundant(byArgs, func(current clause) bool {
			return current.sameResult(*fallback)
		}, *fallback)
	}

	// The keys groupByArgs computed are kept with the clauses, so sorting
	// them builds none again.
	result := make([]keyedClause, 0, len(order))

	for _, key := range order {
		current := byArgs[key]
		if current.sameResult(*fallback) {
			continue
		}

		result = append(result, keyedClause{key: key, clause: current})
	}

	result = pruneDominated(result)

	return sortKeyedClauses(make([]clause, 0, len(result)), result)
}

// pruneDominated drops clauses that can never decide a call under the
// working model: a clause whose filter another clause's filter covers (see
// argsCover) matches only calls the other matches too, and since the least
// restrictive matching clause wins, it is dead unless it is less restrictive
// than that other clause, also where the two filters cover each other (see
// dominates). Clauses with the same result also collapse into the wider
// one.
//
// Two clauses with the same action whose filters cover each other, such as
// "a0 < 1" and "a0 <= 0", are one clause spelled twice: a merge spells a
// conjoined condition after one input, so the same clause can arrive in two
// spellings, which would be no safe shape together, and the set would
// collapse to an unconditional clause that may permit what both inputs
// deny. Such twins live or die together, since argsCover may see a wider
// filter cover one spelling and not the other, and of live twins one stays:
// of those with the same errno, the one with the smallest key, so that a
// union of two inputs is the same whichever comes first, and of those whose
// errno values differ, one with the errno the earliest clause gives, as the
// leftmost input decides an errno where actions tie (see the Errno values
// section of the package documentation). The clauses come in the order of
// their inputs. Three or more inputs merge pairwise, and their order can
// still change how tight a result is (see the Union section).
//
// It compares every clause against every other one, so past maxPruneClauses
// it is skipped. Pruning only removes clauses that never decide a call, so
// skipping it leaves the result less minimal but unchanged in what it
// permits. Clauses of one result and one condition each, the shape a
// recorded profile's rules mostly take, are pruned by pruneUniform, which
// keeps the same ones comparing fewer pairs.
func pruneDominated(clauses []keyedClause) []keyedClause {
	if len(clauses) > maxPruneClauses {
		return clauses
	}

	if !slices.ContainsFunc(clauses, func(current keyedClause) bool {
		return len(current.clause.args) != 1 || !current.clause.sameResult(clauses[0].clause)
	}) {
		return pruneUniform(clauses)
	}

	return prunePairwise(clauses)
}

// prunePairwise is pruneDominated comparing every clause against every
// other one.
func prunePairwise(clauses []keyedClause) []keyedClause {
	dead := make([]bool, len(clauses))

	for idx, current := range clauses {
		dead[idx] = slices.ContainsFunc(clauses, func(other keyedClause) bool {
			return dominates(other.clause, current.clause)
		})
	}

	// The rank of a clause is the index of the first clause with its result,
	// which orders the errno values of twins by input.
	rank := make([]int, len(clauses))

	for idx, current := range clauses {
		rank[idx] = slices.IndexFunc(clauses, func(other keyedClause) bool {
			return other.clause.sameResult(current.clause)
		})
	}

	kept := make([]keyedClause, 0, len(clauses))

	for idx, current := range clauses {
		if !dead[idx] && !twinGoes(clauses, dead, rank, idx) {
			kept = append(kept, current)
		}
	}

	return kept
}

// pruneUniform is pruneDominated for clauses of one result with one
// condition each, and keeps the clauses it keeps. With one result a clause
// is dead where another covers it and it does not cover that one, and
// every clause has the rank of the first, so of live twins the one with the
// smallest key stays. Clauses on different argument indices never cover
// each other (see argsCover), and of two single equalities on one index
// neither covers the other, since canonical arguments would make them
// exact duplicates, which no caller passes. So only pairs on one index of
// which one is no equality are compared, which for distinct equalities
// next to a few ranges is linear rather than quadratic.
func pruneUniform(clauses []keyedClause) []keyedClause {
	dead, twinPairs := uniformCovers(clauses)

	// A twin goes for a dead twin, and for a live one of a smaller key.
	goes := make([]bool, len(clauses))

	for _, pair := range twinPairs {
		first, second := clauses[pair[0]], clauses[pair[1]]
		goes[pair[0]] = goes[pair[0]] || dead[pair[1]] || second.key < first.key
		goes[pair[1]] = goes[pair[1]] || dead[pair[0]] || first.key < second.key
	}

	kept := make([]keyedClause, 0, len(clauses))

	for idx, current := range clauses {
		if !dead[idx] && !goes[idx] {
			kept = append(kept, current)
		}
	}

	return kept
}

// uniformCovers compares the pairs of clauses pruneUniform compares and
// returns which clauses another strictly covers, and the pairs of clauses
// that cover each other, by index into clauses.
func uniformCovers(clauses []keyedClause) ([]bool, [][2]int) {
	dead := make([]bool, len(clauses))

	var twinPairs [][2]int

	compare := func(first, second int) {
		left, right := clauses[first].clause.args, clauses[second].clause.args
		leftCovers, rightCovers := argsCover(left, right), argsCover(right, left)

		switch {
		case leftCovers && rightCovers:
			twinPairs = append(twinPairs, [2]int{first, second})
		case leftCovers:
			dead[second] = true
		case rightCovers:
			dead[first] = true
		}
	}

	for _, group := range groupByIndex(clauses) {
		for offset, first := range group.others {
			for _, second := range group.others[offset+1:] {
				compare(first, second)
			}

			for _, second := range group.equalities {
				compare(first, second)
			}
		}
	}

	return dead, twinPairs
}

// indexGroup holds the indices, into a list of clauses of one condition
// each, of the clauses on one argument index, by whether they test it for
// equality.
type indexGroup struct{ equalities, others []int }

// groupByIndex groups clauses of one condition each by argument index.
func groupByIndex(clauses []keyedClause) map[uint]*indexGroup {
	byIndex := make(map[uint]*indexGroup)

	for idx, current := range clauses {
		index := current.clause.args[0].Index

		group, ok := byIndex[index]
		if !ok {
			group = &indexGroup{}
			byIndex[index] = group
		}

		if singleEquality(current.clause) {
			group.equalities = append(group.equalities, idx)
		} else {
			group.others = append(group.others, idx)
		}
	}

	return byIndex
}

// twinGoes reports whether the clause at idx goes for a twin (see twins):
// one that is dead, or one that comes first by rank, the input order of its
// errno, and then by key, which is a total order, so that one of live twins
// stays.
func twinGoes(clauses []keyedClause, dead []bool, rank []int, idx int) bool {
	current := clauses[idx]

	for otherIdx, other := range clauses {
		if otherIdx == idx || !twins(other.clause, current.clause) {
			continue
		}

		if dead[otherIdx] || rank[otherIdx] < rank[idx] ||
			rank[otherIdx] == rank[idx] && other.key < current.key {
			return true
		}
	}

	return false
}

// dominates reports whether the clause other makes current dead: other
// covers current and is less restrictive, or strictly covers it with the
// same result, which twins leaves to twinGoes.
//
// Of two clauses whose filters cover each other, such as "a0 < 1" and
// "a0 == 0", the stricter one never decides a call, as groupByArgs keeps
// the looser of two clauses spelled alike. Kept, the pair is a conflict to
// libseccomp, which made the clause set collapse as one, to an
// unconditional clause: a union of "read ALLOW a0 < 1" and "read TRAP a0
// == 0" allowed every read. Dropping it is as sound for an intersection,
// where every clause is at least as restrictive as both inputs on its
// region (see adjustClauses), so the looser one is too, and it matches the
// same calls. That case is defensive: adjustClauses lowers each of two
// such clauses of an intersection to the stricter action of the pair, so
// they arrive as twins (see twins), and no intersection is known to keep
// two with different results. Of two such clauses whose actions differ only
// in errno neither is stricter, and twinGoes picks one.
func dominates(other, current clause) bool {
	if !argsCover(other.args, current.args) {
		return false
	}

	if argsCover(current.args, other.args) {
		return stricter(current, other)
	}

	return current.sameResult(other) || stricter(current, other)
}

// twins reports whether two clauses have the same action, whatever their
// errno values, and filters that cover each other, so that they are one
// clause spelled twice.
func twins(first, second clause) bool {
	return actionsEquivalent(first.action, second.action) &&
		argsCover(first.args, second.args) && argsCover(second.args, first.args)
}

// raiseOverlapsOfRedundant raises every clause that is stricter than a
// redundant clause and may match a call the redundant clause also matches.
// Redundant clauses are dropped from the result, or skipped by runtimes when
// they equal the default, so without this the stricter clause would win
// where both match and the union could deny a call an input permits. A
// raised clause becomes redundant itself, so raising propagates.
//
// A raised clause takes raiseTo, which is what decides the region once the
// redundant clause is gone, and raiseTo satisfies redundant, so a clause is
// raised at most once. Processing the redundant clauses from a work list
// therefore compares each pair at most once, and the closure it computes
// does not depend on map iteration order.
func raiseOverlapsOfRedundant(
	byArgs map[string]clause, redundant func(clause) bool, raiseTo clause,
) {
	pending := make([]string, 0, len(byArgs))

	for key, current := range byArgs {
		if redundant(current) {
			pending = append(pending, key)
		}
	}

	for len(pending) > 0 {
		key := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		current := byArgs[key]

		for otherKey, other := range byArgs {
			if otherKey == key || argsDisjoint(current.args, other.args) ||
				!stricter(other, current) {
				continue
			}

			raised := raiseTo
			raised.errnoRet = merge.ClonePtr(raiseTo.errnoRet)
			raised.args = other.args
			byArgs[otherKey] = raised

			pending = append(pending, otherKey)
		}
	}
}

// stricter reports whether the first clause applies a strictly more
// restrictive action than the second.
func stricter(first, second clause) bool {
	return !actionsEquivalent(first.action, second.action) &&
		actionsEquivalent(moreRestrictive(first.action, second.action), first.action)
}

func defaultClause(profile *specs.LinuxSeccomp) *clause {
	return &clause{
		action:   canonicalAction(profile.DefaultAction),
		errnoRet: runtimeErrno(profile.DefaultAction, profile.DefaultErrnoRet),
	}
}

// mergedNames returns every syscall name of either side once, sorted.
func mergedNames(left, right map[string]*syscallRules) []string {
	names := slices.AppendSeq(make([]string, 0, len(left)+len(right)), maps.Keys(left))
	names = slices.AppendSeq(names, maps.Keys(right))
	slices.Sort(names)

	return slices.Compact(names)
}

// mergeProfileSyscalls merges the syscall entries of two profiles given the
// merged default clause. Entries equal to the merged default are elided.
func (m ruleMerger) mergeProfileSyscalls(
	left, right *specs.LinuxSeccomp,
	mergedDefault *clause,
) []specs.LinuxSyscall {
	leftDef := defaultClause(left)
	rightDef := defaultClause(right)
	leftRules := m.readRules(left.Syscalls, leftDef)
	rightRules := m.readRules(right.Syscalls, rightDef)

	m.forInput(left).settleInputs(leftRules, leftDef)
	m.forInput(right).settleInputs(rightRules, rightDef)

	names := mergedNames(leftRules, rightRules)

	var result []specs.LinuxSyscall

	for _, name := range names {
		fallback, conditional := m.mergeRules(
			leftRules[name], rightRules[name], leftDef, rightDef,
		)
		fallback, conditional = m.finishRules(mergedDefault, fallback, conditional)

		result = appendRules(result, name, mergedDefault, fallback, conditional)
	}

	return m.shareJoinedEntries(result)
}

// readRules reads syscall entries as collectRules reads them, with the
// names of an alias the native architecture lacks read as one syscall (see
// joinedAliases).
func (m ruleMerger) readRules(syscalls []specs.LinuxSyscall, def *clause) map[string]*syscallRules {
	return collectRules(joinAliases(syscalls, joinedAliases(m.native)), def)
}

// appendRules emits the settled rules of one syscall, eliding a fallback
// equal to def.
func appendRules(
	result []specs.LinuxSyscall, name string,
	def, fallback *clause, conditional []clause,
) []specs.LinuxSyscall {
	if fallback != nil && !fallback.sameResult(*def) {
		result = append(result, clauseToSyscall(name, *fallback))
	}

	for _, current := range conditional {
		result = append(result, clauseToSyscall(name, current))
	}

	return result
}

// finishRules turns merged rules into emittable ones. A fallback equal to
// the merged default is elided, and conditional clauses are collapsed
// against whatever applies where they do not match: the fallback, or the
// merged default once the fallback is elided. A fallback that differs from
// the default only by errno is elided as well when a conditional clause with
// a different action remains, since keeping that clause is worth more than
// the errno value: an unconditional entry would force it to collapse.
// Clauses that share the fallback's action fold into it instead, so the
// leftmost errno survives. Whatever remains is settled into a safe shape.
func (m ruleMerger) finishRules(
	mergedDefault, fallback *clause, conditional []clause,
) (*clause, []clause) {
	switch {
	case fallback.sameResult(*mergedDefault):
		fallback = nil
		conditional = m.collapseClauses(conditional, mergedDefault)
	case actionsEquivalent(fallback.action, mergedDefault.action):
		conditional = m.collapseClauses(conditional, fallback)

		if slices.ContainsFunc(conditional, func(current clause) bool {
			return !actionsEquivalent(current.action, fallback.action)
		}) {
			conditional = m.collapseClauses(conditional, mergedDefault)
			fallback = nil
		}
	default:
		conditional = m.collapseClauses(conditional, fallback)
	}

	fallback, conditional = m.resolveMixed(mergedDefault, fallback, conditional)

	return m.settleOutput(mergedDefault, fallback, conditional)
}

// settledSyscalls returns the entries of one profile the way a merge in
// the direction of merger reads them (see readRules and settleInputs), in
// the form a merge result takes: one name per entry, conditional clauses
// sorted, and none another one with the same result covers (see
// pruneCovered). def is the profile default; entries equal to it are
// elided.
func settledSyscalls(
	merger ruleMerger, syscalls []specs.LinuxSyscall, def *clause,
) []specs.LinuxSyscall {
	rules := merger.readRules(syscalls, def)
	merger.settleInputs(rules, def)
	pruneCovered(rules)

	return merger.shareJoinedEntries(syscallsOfRules(rules, def))
}

// pruneCovered drops, from every syscall whose conditional clauses form a
// safe shape, each clause another clause with the same result covers, of
// two that cover each other all but one, as pruneDominated drops them from
// a merge result. libseccomp evaluates a safe shape as the working model
// does, so this changes nothing a call gets, and a profile read on its own
// then takes the form a merge with an input that constrains nothing gives
// it. Outside the safe shapes libseccomp's order may let a covered clause
// decide, so those syscalls are left as they are.
//
// Pruning compares clauses in pairs (see pruneUniform), so a syscall is
// skipped where no clause can cover another (see mayCover), as for the
// distinct equalities a recorded profile is mostly made of, and past
// maxPruneClauses (see pruneDominated). A syscall skipped past that bound
// keeps its covered clauses, which Diff can then report as a change against
// a profile without them, and never hides one.
func pruneCovered(rules map[string]*syscallRules) {
	for _, current := range rules {
		if !mayCover(current.conditional) || !safeShape(current.conditional) {
			continue
		}

		keyed := make([]keyedClause, len(current.conditional))
		for idx, next := range current.conditional {
			keyed[idx] = keyedClause{key: sortedArgsKey(next.args), clause: next}
		}

		keyed = pruneDominated(keyed)
		current.conditional = sortKeyedClauses(make([]clause, 0, len(keyed)), keyed)
	}
}

// mayCover reports whether, of clauses free of exact duplicates, one may
// cover another, which takes two clauses and one that is no single
// equality: a single equality covers only a filter that tests the same
// argument for the same value, and canonical arguments make two such
// clauses exact duplicates.
func mayCover(clauses []clause) bool {
	return len(clauses) > 1 && slices.ContainsFunc(clauses, func(current clause) bool {
		return !singleEquality(current)
	})
}

// syscallsOfRules emits the rules collectRules read, by syscall name, as
// one-name entries.
func syscallsOfRules(rules map[string]*syscallRules, def *clause) []specs.LinuxSyscall {
	var result []specs.LinuxSyscall

	for _, name := range slices.Sorted(maps.Keys(rules)) {
		current := rules[name]
		if current.summary != nil {
			result = appendSummary(result, name, def, current.summary)

			continue
		}

		result = appendRules(
			result, name, def, current.unconditional, sortClauses(current.conditional),
		)
	}

	return result
}

// appendSummary emits the rules of a syscall that was summarized for
// exceeding the collectRules budget, for a caller without a merge direction:
// both clauses a collapse could pick, so that two inputs differing in either
// direction still differ here. A merge never gets this far, since
// settleInputs resolves the summary to the clause its direction picks.
func appendSummary(
	result []specs.LinuxSyscall, name string, def *clause, summary *clauseSummary,
) []specs.LinuxSyscall {
	result = appendRules(result, name, def, &summary.strictest, nil)
	if summary.loosest.sameResult(summary.strictest) {
		return result
	}

	return appendRules(result, name, def, &summary.loosest, nil)
}

func clauseToSyscall(name string, current clause) specs.LinuxSyscall {
	return specs.LinuxSyscall{
		Names:    []string{name},
		Action:   current.action,
		ErrnoRet: outputErrno(current.action, current.errnoRet),
		Args:     slices.Clone(current.args),
	}
}
