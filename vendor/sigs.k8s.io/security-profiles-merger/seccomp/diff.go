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
	"slices"
	"strconv"
	"strings"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"sigs.k8s.io/security-profiles-merger/internal/merge"
	"sigs.k8s.io/security-profiles-merger/spm"
)

// ProfileDiff describes the differences between two seccomp profiles.
type ProfileDiff struct {
	// Equal is true when the profiles compare equal in the normalized form
	// Diff describes (see the Diff section), even where spelled differently.
	Equal bool `json:"equal"`

	// DefaultAction is set when the default actions differ, as written,
	// except that SCMP_ACT_KILL_THREAD is reported as SCMP_ACT_KILL; the two
	// compare equal.
	DefaultAction *ValueDiff[specs.LinuxSeccompAction] `json:"defaultAction,omitempty"`

	// DefaultErrnoRet is set when the default errno return values differ.
	// A nil side stands for no value, which is distinct from 0.
	DefaultErrnoRet *ValueDiff[*uint] `json:"defaultErrnoRet,omitempty"`

	// Architectures is set when the architecture lists differ.
	Architectures *SliceDiff[specs.Arch] `json:"architectures,omitempty"`

	// Flags is set when the flag lists differ.
	Flags *SliceDiff[specs.LinuxSeccompFlag] `json:"flags,omitempty"`

	// ListenerPath is set when the listener paths differ. The empty string
	// stands for no value.
	ListenerPath *ValueDiff[string] `json:"listenerPath,omitempty"`

	// ListenerMetadata is set when the listener metadata differs. The empty
	// string stands for no value.
	ListenerMetadata *ValueDiff[string] `json:"listenerMetadata,omitempty"`

	// Syscalls is set when the syscall rules differ.
	Syscalls *SyscallsDiff `json:"syscalls,omitempty"`
}

// ValueDiff represents a changed value. It is [spm.ValueDiff], which the
// apparmor diff names as well.
type ValueDiff[T any] = spm.ValueDiff[T]

// SliceDiff represents added and removed items in a set-like slice. It is
// [spm.SliceDiff], which the apparmor and landlock diffs name as well.
type SliceDiff[T comparable] = spm.SliceDiff[T]

// SyscallsDiff describes differences in the syscall entries, compared per
// syscall name by the rules a runtime loads for it.
type SyscallsDiff struct {
	// Added holds the rules of the syscalls only the right profile loads
	// rules for, one entry per rule, sorted by name and then as
	// SyscallChange.Left orders the rules of one syscall.
	Added []SyscallEntry `json:"added,omitempty"`
	// Removed holds the rules of the syscalls only the left profile loads
	// rules for, ordered as Added.
	Removed []SyscallEntry `json:"removed,omitempty"`
	// Changed holds the syscalls both profiles load rules for, with
	// different rules, sorted by name.
	Changed []SyscallChange `json:"changed,omitempty"`
}

// SyscallEntry is one rule of one syscall, named. It reads as its
// SyscallDetail, and JSON spells the detail's fields next to the name.
type SyscallEntry struct {
	// Name is the syscall name.
	Name string `json:"name"`

	SyscallDetail //nolint:embeddedstructfieldcheck // second, so that JSON writes the name first
}

// SyscallChange is a syscall both profiles load different rules for.
type SyscallChange struct {
	// Name is the syscall name.
	Name string `json:"name"`
	// Left holds the rules the left profile loads for the syscall in the
	// normal form Diff compares, sorted by their conditions, or in the order
	// a runtime adds them where only that order differs (see the Diff
	// section). For a syscall past the read budget it holds the
	// unconditional rules a collapse could pick instead: its most and its
	// least restrictive action, each unless it equals the default, and one
	// where they agree.
	Left []SyscallDetail `json:"left"`
	// Right holds the rules of the right profile, as Left holds the left's.
	Right []SyscallDetail `json:"right"`
}

// SyscallDetail is one rule of a syscall: the action a call of that name
// gets when it matches every condition of Args.
type SyscallDetail struct {
	// Action is the action of the rule. SCMP_ACT_KILL_THREAD is reported as
	// SCMP_ACT_KILL, as a merge result spells it, since the two compare
	// equal.
	Action specs.LinuxSeccompAction `json:"action"`
	// ErrnoRet is the errno the rule returns, set only for SCMP_ACT_ERRNO
	// and SCMP_ACT_TRACE, where an unset value reads as EPERM (see the
	// Errno values section of the package documentation).
	ErrnoRet *uint `json:"errnoRet,omitempty"`
	// Args are the conditions of the rule, sorted by argument index and
	// written as libseccomp compares them; an unconditional rule has none.
	Args []specs.LinuxSeccompArg `json:"args,omitempty"`
}

// Diff compares two profiles by the rules a runtime loads from them,
// normalized as a merge reads them, so Diff(p, Intersect(p)) reports exactly
// what Intersect settled. It validates nothing, reports a nil profile as an
// *InputError matching ErrNilProfile, Index 0 for left and 1 for right, and
// returns a non-nil diff whenever the error is nil. It compares only the
// fields this package reads ([ErrUnsupportedSpecField]) and implies the
// native architecture of the running program on both sides ([Options.Diff]
// names another). The diff shares no memory with the profiles. See the Diff
// section.
func Diff(left, right *specs.LinuxSeccomp) (*ProfileDiff, error) {
	return ForHost().Diff(left, right)
}

// diffProfiles compares two profiles with the given native architecture
// implied on both sides, or none for the empty Arch: Diff settles nothing
// for the 32-bit and multiplexing architectures, so the empty Arch only
// changes how the architecture lists compare, while the merges and the
// validators read it as any architecture this package knows.
func diffProfiles(native specs.Arch, left, right *specs.LinuxSeccomp) (*ProfileDiff, error) {
	err := merge.RequireNonNil(left, right)
	if err != nil {
		return nil, err
	}

	// The merges read the two names of an alias the native architecture
	// lacks as one syscall (see joinedAliases), and so does Diff.
	joined := joinedAliases(native)
	leftSyscalls := joinAliases(left.Syscalls, joined)
	rightSyscalls := joinAliases(right.Syscalls, joined)

	diff := &ProfileDiff{
		DefaultAction: merge.NewValueDiff(
			defaultClause(left).action, defaultClause(right).action,
		),
		DefaultErrnoRet:  diffDefaultErrnoRet(left, right),
		Architectures:    diffArchitectures(native, left, right),
		Flags:            merge.NewSliceDiff(loadedFlags(left.Flags), loadedFlags(right.Flags)),
		ListenerPath:     merge.NewValueDiff(left.ListenerPath, right.ListenerPath),
		ListenerMetadata: merge.NewValueDiff(left.ListenerMetadata, right.ListenerMetadata),
		Syscalls: diffSyscallLists(
			leftSyscalls, rightSyscalls, defaultClause(left), defaultClause(right),
		),
	}
	if diff.Syscalls != nil {
		reportWrittenErrnos(diff.Syscalls, leftSyscalls, rightSyscalls)
		splitJoinedNames(diff.Syscalls, joined, left.Syscalls, right.Syscalls)
	}
	// Every field but Equal is a pointer, set only where its section
	// differs, so the profiles are equal where none is set.
	var unchanged ProfileDiff

	diff.Equal = *diff == unchanged

	return diff, nil
}

// diffSyscallLists compares two syscall lists by the rules a runtime loads
// from them and returns nil when they are equal. leftDef and rightDef are
// the profile defaults.
//
// A syscall read in summarized form on either side (see collectRules) is
// listed by the two clauses a collapse could pick rather than by its rules,
// and two different rule sets can share those. Such a syscall is compared by
// the entries that load its rules instead, which costs one pass over the
// lists: it is equal only where both sides load the same rules from entries
// in the same order.
//
// A syscall whose rules may depend on the order they are added in (see
// orderSensitiveGroups) is compared by its rules and by that order: the same
// rules added in another order can decide a call differently there, so
// comparing by rule set alone would call such profiles equal.
func diffSyscallLists(left, right []specs.LinuxSyscall, leftDef, rightDef *clause) *SyscallsDiff {
	summarized := summarizedNames(left, leftDef)
	for name := range summarizedNames(right, rightDef) {
		if summarized == nil {
			summarized = make(map[string]bool)
		}

		summarized[name] = true
	}

	var leftLoaded, rightLoaded map[string][]string
	if len(summarized) > 0 {
		leftLoaded = loadedEntryKeys(left, leftDef, summarized)
		rightLoaded = loadedEntryKeys(right, rightDef, summarized)
	}

	leftRules := collectRules(left, leftDef)
	rightRules := collectRules(right, rightDef)

	// A syscall in a safe shape is never order sensitive, so pruning it
	// leaves orderedGroups as it finds it.
	pruneCovered(leftRules)
	pruneCovered(rightRules)

	ordered := orderedGroups(leftRules, nil)
	ordered = orderedGroups(rightRules, ordered)

	var leftOrder, rightOrder addOrder
	if len(ordered) > 0 {
		leftOrder = loadAddOrder(leftRules, ordered)
		rightOrder = loadAddOrder(rightRules, ordered)
	}

	return diffSyscallMaps(
		buildSyscallMap(leftRules, leftDef), buildSyscallMap(rightRules, rightDef),
		func(name string) syscallVerdict {
			switch {
			case summarized[name]:
				return syscallVerdict{
					done: true, equal: slices.Equal(leftLoaded[name], rightLoaded[name]),
				}
			case ordered[name] != nil && !leftOrder.sameGroups(rightOrder, name, ordered[name]):
				return syscallVerdict{
					done: true,
					left: leftOrder.entries(name), right: rightOrder.entries(name),
				}
			default:
				return syscallVerdict{}
			}
		},
	)
}

// splitJoinedNames reports the changes of a syscall Diff read as the two
// names of an alias (see joinedAliases), which it finds under the first
// name, under the names the profiles use: each name of the alias some entry
// of either profile names, so that a diff never names a syscall neither
// profile does.
func splitJoinedNames(diff *SyscallsDiff, joined []syscallAlias, left, right []specs.LinuxSyscall) {
	split := false

	for _, alias := range joined {
		used := alias.usedNames(left, right)

		if slices.Equal(used, alias[:1]) {
			continue
		}

		split = true
		diff.Removed = renameEntries(diff.Removed, alias[0], used)
		diff.Added = renameEntries(diff.Added, alias[0], used)

		var changed []SyscallChange

		for _, change := range diff.Changed {
			if change.Name != alias[0] {
				changed = append(changed, change)

				continue
			}

			for _, name := range used {
				changed = append(changed, SyscallChange{
					Name:  name,
					Left:  cloneDetails(change.Left),
					Right: cloneDetails(change.Right),
				})
			}
		}

		diff.Changed = changed
	}

	if split {
		byName := func(first, second SyscallEntry) int { return cmp.Compare(first.Name, second.Name) }
		slices.SortStableFunc(diff.Removed, byName)
		slices.SortStableFunc(diff.Added, byName)
		slices.SortStableFunc(diff.Changed, func(first, second SyscallChange) int {
			return cmp.Compare(first.Name, second.Name)
		})
	}
}

// renameEntries returns the entries with each entry of the syscall name
// repeated under every one of names, in their place.
func renameEntries(entries []SyscallEntry, name string, names []string) []SyscallEntry {
	var renamed []SyscallEntry

	for _, entry := range entries {
		if entry.Name != name {
			renamed = append(renamed, entry)

			continue
		}

		for _, each := range names {
			renamed = append(renamed, SyscallEntry{
				Name: each, SyscallDetail: cloneDetail(entry.SyscallDetail),
			})
		}
	}

	return renamed
}

// cloneDetails returns a copy of rules sharing no memory with them, so that
// the copies a diff reports under two names are its own.
func cloneDetails(details []SyscallDetail) []SyscallDetail {
	if details == nil {
		return nil
	}

	cloned := make([]SyscallDetail, len(details))

	for idx, detail := range details {
		cloned[idx] = cloneDetail(detail)
	}

	return cloned
}

// cloneDetail returns a copy of a rule sharing no memory with it.
func cloneDetail(detail SyscallDetail) SyscallDetail {
	return SyscallDetail{
		Action:   detail.Action,
		ErrnoRet: merge.ClonePtr(detail.ErrnoRet),
		Args:     slices.Clone(detail.Args),
	}
}

// syscallVerdict is how a syscall compares where its entries are not
// compared as the rules they load: done reports whether it is decided so,
// equal whether the two sides are then equal, and left and right, where
// set, the entries to report instead of its rules.
type syscallVerdict struct {
	done, equal bool
	left, right []SyscallEntry
}

// loadedEntryKeys returns, for every syscall in only, a key per entry that
// loads rules for it, in entry order and without exact repeats, which add
// nothing at load time. Two lists with the same keys for a syscall load the
// same rules for it in the same order.
func loadedEntryKeys(
	syscalls []specs.LinuxSyscall, def *clause, only map[string]bool,
) map[string][]string {
	keys := make(map[string][]string)
	// Keyed by syscall name and entry key.
	seen := make(map[[2]string]struct{})

	for idx := range syscalls {
		entry := &syscalls[idx]
		if !slices.ContainsFunc(entry.Names, func(name string) bool { return only[name] }) {
			continue
		}

		key := loadedEntryKey(entry, def)
		if key == "" {
			continue
		}

		for _, name := range entry.Names {
			if !only[name] {
				continue
			}

			if _, dup := seen[[2]string{name, key}]; dup {
				continue
			}

			seen[[2]string{name, key}] = struct{}{}
			keys[name] = append(keys[name], key)
		}
	}

	return keys
}

// loadedEntryKey formats the rules one entry loads for each of its names,
// or returns "" when it loads none. Every clause key is prefixed with its
// length, since Diff does not validate and an action may hold any byte.
func loadedEntryKey(entry *specs.LinuxSyscall, def *clause) string {
	var builder strings.Builder

	for _, next := range entryClauses(entry) {
		if next.sameResult(*def) {
			continue
		}

		key := clauseKey(next)
		builder.WriteString(strconv.Itoa(len(key)))
		builder.WriteByte(':')
		builder.WriteString(key)
	}

	return builder.String()
}

// diffSyscallMaps compares two per-name entry maps and returns nil when
// they are equal. decided reports for a syscall whether its entries are
// compared some other way, and if so whether they are equal and which
// entries to report (see syscallVerdict).
func diffSyscallMaps(
	leftMap, rightMap map[string][]SyscallEntry,
	decided func(name string) syscallVerdict,
) *SyscallsDiff {
	var result SyscallsDiff

	added, removed, both := merge.SortedKeyDiff(leftMap, rightMap)

	for _, name := range removed {
		result.Removed = append(result.Removed, leftMap[name]...)
	}

	for _, name := range added {
		result.Added = append(result.Added, rightMap[name]...)
	}

	for _, name := range both {
		if change, ok := changedSyscall(name, leftMap[name], rightMap[name], decided); ok {
			result.Changed = append(result.Changed, change)
		}
	}

	if len(result.Added) == 0 &&
		len(result.Removed) == 0 &&
		len(result.Changed) == 0 {
		return nil
	}

	return &result
}

// changedSyscall compares the entries both sides hold for a syscall, as
// diffSyscallMaps describes, and returns the change and true where they
// differ.
func changedSyscall(
	name string, leftEntries, rightEntries []SyscallEntry,
	decided func(name string) syscallVerdict,
) (SyscallChange, bool) {
	var equal bool
	if verdict := decided(name); verdict.done {
		equal = verdict.equal

		if verdict.left != nil || verdict.right != nil {
			leftEntries, rightEntries = verdict.left, verdict.right
		}
	} else {
		equal = equalSyscallEntrySlices(leftEntries, rightEntries)
	}

	if equal {
		return SyscallChange{}, false
	}

	return SyscallChange{
		Name:  name,
		Left:  entriesToDetails(leftEntries),
		Right: entriesToDetails(rightEntries),
	}, true
}

// diffDefaultErrnoRet compares the default errno values as a runtime loads
// them (see errnoMask), and reports them as reportedErrno spells them.
func diffDefaultErrnoRet(left, right *specs.LinuxSeccomp) *ValueDiff[*uint] {
	leftRet := outputErrno(left.DefaultAction, left.DefaultErrnoRet)
	rightRet := outputErrno(right.DefaultAction, right.DefaultErrnoRet)

	if compareUintPtr(leftRet, rightRet) == 0 {
		return nil
	}

	return &ValueDiff[*uint]{
		Left:  reportedErrno(left.DefaultAction, left.DefaultErrnoRet),
		Right: reportedErrno(right.DefaultAction, right.DefaultErrnoRet),
	}
}

// reportedErrno returns an errno as a diff reports it: as outputErrno
// spells it, except that a value wider than the bits a runtime loads is
// reported as written. Diff validates nothing, and the lower 16 bits of
// 99999 are 34463, a value the profile never names and which no runtime
// returns either, since the kernel caps an errno at 4095: shown as loaded,
// the diff would point at a number nobody can find in the profile.
func reportedErrno(action specs.LinuxSeccompAction, ret *uint) *uint {
	if errnoSignificant(action) && ret != nil && *ret > errnoMask {
		return merge.ClonePtr(ret)
	}

	return outputErrno(action, ret)
}

// wideErrno is an errno the entries of a syscall write wider than the bits
// a runtime loads, for an action and the value it loads as. mixed reports
// entries that write different values for them.
type wideErrno struct {
	action  specs.LinuxSeccompAction
	loaded  uint
	written uint
	mixed   bool
}

// reportWrittenErrnos replaces each errno of a rule in the diff that the
// entries of the profiles write wider than the bits a runtime loads with
// the value they write, as reportedErrno does for the default: the rules
// carry the loaded value, which for 99999 is 34463, a number the profile
// never names. A rule does not say which entries it came from, so the value
// is replaced only where every entry of the syscall with that action and
// loaded value, in both profiles, writes the same one; where they differ,
// as 99999 and 34463 do, the rule keeps the loaded value on both sides, so
// that spelling alone never reads as a change. The arguments are left out
// of the match, since Diff rewrites them into the form libseccomp
// compares, which only shows the loaded value more often.
func reportWrittenErrnos(diff *SyscallsDiff, left, right []specs.LinuxSyscall) {
	wide := wideErrnos(slices.Concat(left, right))
	if wide == nil {
		return
	}

	writeErrnos(diff.Removed, wide)
	writeErrnos(diff.Added, wide)

	for idx := range diff.Changed {
		writeDetailErrnos(diff.Changed[idx].Name, diff.Changed[idx].Left, wide)
		writeDetailErrnos(diff.Changed[idx].Name, diff.Changed[idx].Right, wide)
	}
}

// wideErrnos returns, per syscall name, the errno each action and loaded
// value is written with in the entries, keeping only those all entries
// write alike and wider than the bits a runtime loads, or nil for none.
func wideErrnos(syscalls []specs.LinuxSyscall) map[string][]wideErrno {
	if !slices.ContainsFunc(syscalls, func(entry specs.LinuxSyscall) bool {
		return errnoSignificant(entry.Action) && entry.ErrnoRet != nil &&
			*entry.ErrnoRet > errnoMask
	}) {
		return nil
	}

	written := make(map[string][]wideErrno)

	for idx := range syscalls {
		entry := &syscalls[idx]
		if !errnoSignificant(entry.Action) {
			continue
		}

		value := defaultErrno
		if entry.ErrnoRet != nil {
			value = *entry.ErrnoRet
		}

		loaded := *runtimeErrno(entry.Action, entry.ErrnoRet)

		for _, name := range entry.Names {
			written[name] = addWrittenErrno(written[name], entry.Action, loaded, value)
		}
	}

	for name, errnos := range written {
		written[name] = slices.DeleteFunc(errnos, func(each wideErrno) bool {
			return each.mixed || each.written <= errnoMask
		})
	}

	return written
}

// addWrittenErrno records in errnos that an entry with the action writes
// value, which loads as loaded, and returns errnos: as a new record, or by
// marking the record of an equivalent action and loaded value mixed where
// it holds another value.
func addWrittenErrno(
	errnos []wideErrno, action specs.LinuxSeccompAction, loaded, value uint,
) []wideErrno {
	found := findWideErrno(errnos, action, loaded)
	if found == nil {
		return append(errnos, wideErrno{action: action, loaded: loaded, written: value})
	}

	if found.written != value {
		found.mixed = true
	}

	return errnos
}

// findWideErrno returns the errno of errnos recorded for an action
// equivalent to the given one and the loaded value, or nil.
func findWideErrno(
	errnos []wideErrno, action specs.LinuxSeccompAction, loaded uint,
) *wideErrno {
	for idx := range errnos {
		if errnos[idx].loaded == loaded && actionsEquivalent(errnos[idx].action, action) {
			return &errnos[idx]
		}
	}

	return nil
}

// writeErrnos replaces the errno of each entry the wide errnos of its
// profile name (see reportWrittenErrnos).
func writeErrnos(entries []SyscallEntry, wide map[string][]wideErrno) {
	for idx := range entries {
		writeDetailErrno(&entries[idx].SyscallDetail, wide[entries[idx].Name])
	}
}

// writeDetailErrnos replaces the errno of each rule of the named syscall
// the wide errnos of its profile name (see reportWrittenErrnos).
func writeDetailErrnos(name string, details []SyscallDetail, wide map[string][]wideErrno) {
	for idx := range details {
		writeDetailErrno(&details[idx], wide[name])
	}
}

// writeDetailErrno replaces the errno of a rule with the one errnos record
// for its action and loaded value, if any.
func writeDetailErrno(detail *SyscallDetail, errnos []wideErrno) {
	if len(errnos) == 0 || !errnoSignificant(detail.Action) {
		return
	}

	found := findWideErrno(errnos, detail.Action, *runtimeErrno(detail.Action, detail.ErrnoRet))
	if found != nil {
		written := found.written
		detail.ErrnoRet = &written
	}
}

// diffArchitectures compares the architecture lists with the given native
// architecture implied on both sides, as runtimes always cover it: a
// profile that lists it and one that does not are the same to the runtime.
func diffArchitectures(native specs.Arch, left, right *specs.LinuxSeccomp) *SliceDiff[specs.Arch] {
	return merge.NewSliceDiff(
		withoutNative(native, left.Architectures),
		withoutNative(native, right.Architectures),
	)
}

func withoutNative(native specs.Arch, archs []specs.Arch) []specs.Arch {
	if native == "" {
		return archs
	}

	return slices.DeleteFunc(slices.Clone(archs), func(arch specs.Arch) bool {
		return arch == native
	})
}

// buildSyscallMap turns the rules collectRules read from syscall entries
// into the per-name entries a runtime loads from them, in the normal form
// the merge produces. def is the profile default. No name holds an entry
// twice: collectRules drops exact duplicates and an unconditional rule hides
// the conditional ones, and a summarized syscall lists its two clauses only
// where they differ.
func buildSyscallMap(
	rules map[string]*syscallRules, def *clause,
) map[string][]SyscallEntry {
	result := make(map[string][]SyscallEntry)

	for _, syscall := range syscallsOfRules(rules, def) {
		for _, name := range syscall.Names {
			result[name] = append(result[name], syscallEntry(name, syscall))
		}
	}

	return result
}

// syscallEntry returns the rule of an entry the merge emits as the entry
// of the syscall name.
func syscallEntry(name string, syscall specs.LinuxSyscall) SyscallEntry {
	return SyscallEntry{Name: name, SyscallDetail: SyscallDetail{
		Action: syscall.Action, ErrnoRet: syscall.ErrnoRet, Args: syscall.Args,
	}}
}

func entriesToDetails(entries []SyscallEntry) []SyscallDetail {
	details := make([]SyscallDetail, 0, len(entries))

	for _, entry := range entries {
		details = append(details, entry.SyscallDetail)
	}

	return details
}

// equalSyscallEntrySlices reports whether two lists hold the same entries,
// in any order.
func equalSyscallEntrySlices(left, right []SyscallEntry) bool {
	if len(left) != len(right) {
		return false
	}

	left, right = slices.Clone(left), slices.Clone(right)
	slices.SortFunc(left, compareSyscallEntry)
	slices.SortFunc(right, compareSyscallEntry)

	return slices.EqualFunc(left, right, func(first, second SyscallEntry) bool {
		return compareSyscallEntry(first, second) == 0
	})
}

// compareSyscallEntry orders entries by action, errno and arguments, which
// are all an entry of one name holds, so it returns 0 exactly when the two
// are equal.
func compareSyscallEntry(left, right SyscallEntry) int {
	return cmp.Or(
		cmp.Compare(left.Action, right.Action),
		compareUintPtr(left.ErrnoRet, right.ErrnoRet),
		slices.CompareFunc(left.Args, right.Args, compareArg),
	)
}

// compareUintPtr orders optional values, no value first.
func compareUintPtr(left, right *uint) int {
	if left == nil && right == nil {
		return 0
	}

	if left == nil {
		return -1
	}

	if right == nil {
		return 1
	}

	return cmp.Compare(*left, *right)
}

// String returns the diff as a single line, quoted and bounded as
// FormatProfile is, for logging. It has a value receiver, so a ProfileDiff
// and a pointer to one both implement fmt.Stringer; through a nil pointer
// it panics.
func (d ProfileDiff) String() string {
	if d.Equal {
		return "Diff{equal}"
	}

	return merge.Framed("Diff{", func(out *merge.Builder, parts *merge.Joiner) {
		writeScalarDiffs(out, parts, &d)
		writeSliceDiffs(out, parts, &d)
		writeListenerDiffs(out, parts, &d)

		if d.Syscalls != nil {
			writeSyscallsDiff(out, parts, d.Syscalls)
		}
	})
}

func writeScalarDiffs(out *merge.Builder, parts *merge.Joiner, diff *ProfileDiff) {
	if diff.DefaultAction != nil {
		parts.Next()
		out.WriteString("default:")
		out.WriteText(string(diff.DefaultAction.Left))
		out.WriteString("->")
		out.WriteText(string(diff.DefaultAction.Right))
	}

	if diff.DefaultErrnoRet != nil {
		parts.Next()
		out.WriteString("defaultErrno:")
		writeUintPtr(out, diff.DefaultErrnoRet.Left)
		out.WriteString("->")
		writeUintPtr(out, diff.DefaultErrnoRet.Right)
	}
}

func writeSliceDiffs(out *merge.Builder, parts *merge.Joiner, diff *ProfileDiff) {
	merge.WriteSliceDiff(out, parts, "arch", diff.Architectures)
	merge.WriteSliceDiff(out, parts, "flags", diff.Flags)
}

func writeListenerDiffs(out *merge.Builder, parts *merge.Joiner, diff *ProfileDiff) {
	if diff.ListenerPath != nil {
		parts.Next()
		out.WriteString("listener:")
		writeTextOrNone(out, diff.ListenerPath.Left)
		out.WriteString("->")
		writeTextOrNone(out, diff.ListenerPath.Right)
	}

	if diff.ListenerMetadata != nil {
		parts.Next()
		out.WriteString("listenerMeta:")
		writeTextOrNone(out, diff.ListenerMetadata.Left)
		out.WriteString("->")
		writeTextOrNone(out, diff.ListenerMetadata.Right)
	}
}

func writeUintPtr(out *merge.Builder, val *uint) {
	if val == nil {
		out.WriteString("<nil>")

		return
	}

	out.WriteUint(uint64(*val))
}

func writeTextOrNone(out *merge.Builder, str string) {
	if str == "" {
		out.WriteString("<none>")

		return
	}

	out.WriteText(str)
}

func writeSyscallsDiff(out *merge.Builder, parts *merge.Joiner, syscallsDiff *SyscallsDiff) {
	writeEntries(out, parts, "-", syscallsDiff.Removed)
	writeEntries(out, parts, "+", syscallsDiff.Added)

	merge.WriteParts(out, parts, len(syscallsDiff.Changed), "~", func(idx int) {
		change := &syscallsDiff.Changed[idx]

		out.WriteText(change.Name)
		out.WriteString(":")
		writeDetails(out, change.Left)
		out.WriteString("->")
		writeDetails(out, change.Right)
	})
}

// writeEntries writes each entry as its String method renders it, after
// sign.
func writeEntries(out *merge.Builder, parts *merge.Joiner, sign string, entries []SyscallEntry) {
	merge.WriteParts(out, parts, len(entries), sign, func(idx int) { entries[idx].writeTo(out) })
}

func writeDetails(out *merge.Builder, details []SyscallDetail) {
	for idx := range details {
		if out.Full() {
			return
		}

		if idx > 0 {
			out.WriteString(",")
		}

		details[idx].writeTo(out)
	}
}
