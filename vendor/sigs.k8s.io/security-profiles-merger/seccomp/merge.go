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
	"errors"
	"fmt"
	"maps"
	"slices"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"sigs.k8s.io/security-profiles-merger/internal/merge"
	"sigs.k8s.io/security-profiles-merger/spm"
)

var (
	// ErrNoProfiles is returned by Intersect and Union when no profiles are
	// given.
	ErrNoProfiles = spm.ErrNoProfiles
	// ErrNilProfile is returned for a nil profile.
	ErrNilProfile = spm.ErrNilProfile
	// ErrMoreProblems is spm.ErrMoreProblems, matched by a validation
	// report that left failures out.
	ErrMoreProblems = spm.ErrMoreProblems
	// ErrInexactBaseline is spm.ErrInexactBaseline, returned by
	// ValidateBaseline for a baseline the merge does not read exactly.
	ErrInexactBaseline = spm.ErrInexactBaseline
	// ErrArtifactTooLarge is spm.ErrArtifactTooLarge, matched by every
	// error this package returns for a profile past a MaxArtifact limit.
	ErrArtifactTooLarge = spm.ErrArtifactTooLarge
	// ErrNotifyWithoutListener is returned by Validate for a profile that
	// applies SCMP_ACT_NOTIFY without a listenerPath, which runc refuses to
	// load, and by Intersect and Union for a result that would.
	ErrNotifyWithoutListener = errors.New("SCMP_ACT_NOTIFY without a listener")
	// ErrConflictingListener is returned by Intersect and Union when inputs
	// name different listeners, and by Intersect when an input after the
	// first sets listenerPath or listenerMetadata other than exactly as the
	// first does; both come in an *InputError naming the later input, with a
	// message naming the other listener. See the Listener section.
	ErrConflictingListener = errors.New("conflicting listeners")
	// ErrLostRules is returned by Union, and by Intersect where it cannot
	// drop an architecture, when libseccomp would lose the rules of a
	// covered architecture of the result, and by ValidateArtifact and
	// ValidateStrict for a profile with that shape as written. See the
	// Architectures section.
	ErrLostRules = errors.New("libseccomp would lose the rules of an architecture")
)

// InputError is [spm.InputError], which a merge or Diff returns for one of
// its inputs, naming it by position; spm.InputError lists its fields and the
// errors a merge returns without it. Here it also wraps
// ErrConflictingListener for the later of two inputs naming different
// listeners.
type InputError = spm.InputError

// Intersect merges profiles so that the result permits a call only if every
// input permits it, as runc and crun load them through libseccomp; where the
// exact intersection is not expressible or not a safe shape, the calls get
// the more restrictive action, so it never permits a call any input denies.
// This is the merge KEP-6061 defines for a CRI runtime combining an
// artifact with its baseline.
//
// Every input is checked with Validate, and each that fails is reported as
// an *InputError naming its index. Pass the baseline first: errno ties go to
// the earlier input, and a later input may name a listener only as the
// first does ([ErrConflictingListener]). Intersect refuses an input listing an
// architecture of another byte order than the native one, or of both where
// that is not known ([ErrMixedByteOrder]), and a result that would notify
// without a listener ([ErrNotifyWithoutListener]). It may drop 32-bit and
// multiplexing architectures, and one whose rules libseccomp would lose;
// where it can drop none it refuses with [ErrLostRules].
//
// The result depends on the native architecture of the running program
// ([Options.Intersect] names another node). It passes Validate but often
// not ValidateStrict, equal inputs give equal output, and it shares no
// memory with the inputs, a single input included. The guarantee holds for
// libseccomp 2.5.4 to 2.6.2 and the syscalls they know; the Evaluation
// model and Intersection sections give the model and its limits. See also
// the Errno values, Architectures, Flags, Listener, Single profiles and Cost
// bounds sections.
func Intersect(profiles ...*specs.LinuxSeccomp) (*specs.LinuxSeccomp, error) {
	return ForHost().Intersect(profiles...)
}

// Union merges profiles so that the result permits a call if any input
// permits it, judged as Intersect judges it; where the exact union is not
// expressible or not a safe shape, the result over-approximates in the
// permissive direction, so it never denies a call any input permits. This is
// the merge the Security Profiles Operator uses to combine recorded
// profiles.
//
// Inputs are validated and errno ties broken as for Intersect, but any input
// may name the listener; inputs naming different listeners are refused with
// [ErrConflictingListener]. Union never drops an architecture an input
// covers: it collapses the affected syscalls of a 32-bit or multiplexing
// architecture to their least restrictive action, refuses a result whose
// rules libseccomp would lose with [ErrLostRules], and keeps an architecture
// of another byte order, so the result fails to load as that input does.
//
// The result depends on the native architecture ([Options.Union]). It
// passes Validate and shares no memory with the inputs, as Intersect's
// does, and its guarantee has the limits of Intersect's. See the Union,
// Errno values, Architectures, Flags, Listener, Single profiles and Cost
// bounds sections.
func Union(profiles ...*specs.LinuxSeccomp) (*specs.LinuxSeccomp, error) {
	return ForHost().Union(profiles...)
}

// ValidateBaseline checks a baseline once, where a runtime loads its
// configuration: Validate, then a Diff of the baseline against Intersect of
// the baseline alone and against its intersection with a profile allowing
// every call on the same architectures, with the same flags and listener.
// An unequal diff names what every merge with that baseline takes away
// whatever the artifact says, and is reported with [ErrInexactBaseline] and
// the diff in the message; treat any error as a misconfigured node. On a
// PowerPC node, or one not known, an artifact can avoid what the diff names
// for switch_endian and sys_debug_setcontext (see the Architectures
// section).
//
// Failures of Validate are returned as Validate returns them, and merge
// errors without an InputError. It depends on the native architecture as
// Intersect does ([Options.ValidateBaseline]). See the Diff section, and
// https://github.com/kubernetes-sigs/security-profiles-merger/blob/main/docs/integration.md#the-runtime-flow
// for where it fits.
func ValidateBaseline(baseline *specs.LinuxSeccomp) error {
	return ForHost().ValidateBaseline(baseline)
}

func foldProfiles(
	profiles []*specs.LinuxSeccomp, rules ruleMerger,
) (*specs.LinuxSeccomp, error) {
	err := merge.CheckInputs(profiles, func(_ int, profile *specs.LinuxSeccomp) error {
		return rules.checkInput(profile)
	})
	if err != nil {
		return nil, err
	}

	err = checkListeners(profiles, rules.intersect)
	if err != nil {
		return nil, err
	}

	result, err := merge.Fold(
		profiles,
		func(left, right *specs.LinuxSeccomp) (*specs.LinuxSeccomp, error) {
			return mergeTwo(left, right, rules), nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("merge: %w", err)
	}

	if len(profiles) == 1 {
		// A single profile is normalized to the form a merge result takes,
		// which is also the form Diff compares, and which is a copy rather
		// than the caller's profile.
		result = normalizeProfile(result, rules)
	}

	rules.unifyResultAliases(result)

	err = rules.settleAbsentSyscalls(result)
	if err != nil {
		return nil, fmt.Errorf("merge: %w", err)
	}

	err = resolveListener(result)
	if err != nil {
		return nil, err
	}

	result.Flags = flagsForListener(
		result.Flags, result.ListenerPath, notifiesAny(result.Syscalls),
	)

	result.Syscalls = regroupSyscalls(result.Syscalls)

	slices.Sort(result.Architectures)
	slices.Sort(result.Flags)

	return result, nil
}

// checkInput runs Validate on an input, and for intersection on a node
// whose native architecture is known, refuses an architecture of another
// byte order than the node's (ErrMixedByteOrder). libseccomp keeps one byte
// order per filter, so the input fails to load on the node, and dropping the
// architecture would give a result that loads where no input does, under
// rules written for another architecture. Union, which never drops an
// architecture an input covers, keeps it in its result, which then fails to
// load as the input does. Where the native architecture is not known,
// either byte order may be native, and intersection refuses an input
// listing architectures of both, which loads on no node. Its result lists
// only architectures every input lists, so it mixes them neither.
func (m ruleMerger) checkInput(profile *specs.LinuxSeccomp) error {
	err := Validate(profile)
	if err != nil || !m.intersect {
		return err
	}

	var errs merge.Collector

	validateByteOrder(&errs, profile, m.native)

	return errs.Err()
}

// normalizeProfile returns the profile in the form a merge result takes,
// without merging it against anything: syscall entries are reduced to what
// a runtime loads from them and settled for the merge direction (see
// settledSyscalls), errno values and SCMP_ACT_KILL_THREAD are spelled
// canonically, and the other fields are copied.
//
// The rules it keeps are the ones the profile loads, so they mean on a
// 32-bit architecture what the profile means there, and need no narrow
// reading. Its multiplexer path is settled like a merge result's, since a
// syscall the settling collapses changes that path as well.
func normalizeProfile(profile *specs.LinuxSeccomp, rules ruleMerger) *specs.LinuxSeccomp {
	def := defaultClause(profile)

	normalized := &specs.LinuxSeccomp{
		DefaultAction:    def.action,
		DefaultErrnoRet:  outputErrno(def.action, def.errnoRet),
		Architectures:    merge.DeduplicateSlice(profile.Architectures),
		Flags:            normalizeFlags(profile.Flags),
		ListenerPath:     profile.ListenerPath,
		ListenerMetadata: profile.ListenerMetadata,
		Syscalls:         nil,
	}

	rules.settleWithArchitectures(normalized, def, func(rules ruleMerger) []specs.LinuxSyscall {
		return settledSyscalls(rules, profile.Syscalls, def)
	}, profile)

	return normalized
}

func mergeTwo(
	left, right *specs.LinuxSeccomp,
	rules ruleMerger,
) *specs.LinuxSeccomp {
	// The merged default follows the same tie-break as every other clause:
	// the left side wins when the actions are equivalent, so its errno
	// survives.
	mergedDefault := rules.pickClause(*defaultClause(left), *defaultClause(right))

	// The listener comes from the first profile that provides one rather
	// than from the left unconditionally: the action lattice can carry
	// SCMP_ACT_NOTIFY in from the right, and a result that notifies without
	// a listenerPath does not load, however loadable both inputs were.
	// checkListeners has refused inputs naming different listeners, so
	// where both sides set one it is the same. Intersect never gets past it
	// with a listener on the right that the left lacks: the left holds the
	// first input.
	listener := left
	if left.ListenerPath == "" && right.ListenerPath != "" {
		listener = right
	}

	merged := &specs.LinuxSeccomp{
		DefaultAction:    mergedDefault.action,
		DefaultErrnoRet:  outputErrno(mergedDefault.action, mergedDefault.errnoRet),
		ListenerPath:     listener.ListenerPath,
		ListenerMetadata: listener.ListenerMetadata,
	}

	merged.Flags = mergeFlags(left.Flags, right.Flags, rules.intersect, listener == left)

	if rules.intersect {
		merged.Architectures = merge.IntersectSlice(left.Architectures, right.Architectures)
	} else {
		merged.Architectures = merge.UnionSlice(left.Architectures, right.Architectures)
	}

	if coversAny(merged.Architectures, narrowArchitectures, rules.native) &&
		(loadsWideValue(left) || loadsWideValue(right)) {
		if rules.intersect && !mayBeNative(narrowArchitectures, rules.native) {
			merged.Architectures = withoutAny(merged.Architectures, narrowArchitectures)
		} else {
			rules.narrow = true
		}
	}

	rules.settleWithArchitectures(
		merged,
		&mergedDefault,
		func(rules ruleMerger) []specs.LinuxSyscall {
			return rules.mergeProfileSyscalls(left, right, &mergedDefault)
		},
		left,
		right,
	)

	return merged
}

// settleWithArchitectures builds the syscalls of a result with build and
// settles its architectures (see settleArchitectures). Whether the result
// covers a 32-bit architecture decides how build reads some syscalls (see
// ruleMerger.narrowCovered), and settling the architectures may drop the
// 32-bit ones, so where it does, the syscalls are built again as a filter
// without them loads them, and the architectures settled again. The list
// only shrinks, so the second pass covers no 32-bit architecture either.
func (m ruleMerger) settleWithArchitectures(
	result *specs.LinuxSeccomp, def *clause,
	build func(rules ruleMerger) []specs.LinuxSyscall, inputs ...*specs.LinuxSeccomp,
) {
	m.narrowCovered = coversAny(result.Architectures, narrowArchitectures, m.native)
	result.Syscalls = build(m)
	m.settleArchitectures(result, def, inputs...)

	if m.narrowCovered && !coversAny(result.Architectures, narrowArchitectures, m.native) {
		m.narrowCovered = false
		result.Syscalls = build(m)
		m.settleArchitectures(result, def, inputs...)
	}
}

// settleArchitectures makes a result safe on the multiplexer path of the
// socket and SysV IPC syscalls when it covers an architecture that
// multiplexes them (see settleMultiplexed). Where it has to settle anything,
// intersection drops the multiplexing architectures from the list instead
// when it can, which is when the native architecture does not multiplex:
// the filter then applies libseccomp's action for an unlisted architecture,
// SCMP_ACT_KILL, to every call of those architectures, as the plain
// intersection of the lists does for an architecture one input leaves out,
// and keeps the rules of the syscalls intact for the architectures that
// remain. A collapse would deny or allow a syscall everywhere to settle one
// architecture. Union cannot drop an architecture an input covers, so it
// collapses.
func (m ruleMerger) settleArchitectures(
	result *specs.LinuxSeccomp, def *clause, inputs ...*specs.LinuxSeccomp,
) {
	if !coversAny(result.Architectures, multiplexingArchitectures, m.native) {
		return
	}

	read := make([]multiplexInput, 0, len(inputs))
	for _, input := range inputs {
		read = append(read, newMultiplexInput(input.Syscalls, defaultClause(input)))
	}

	settled, unsafe := m.settleMultiplexed(result.Syscalls, def, read)
	if unsafe && m.intersect && !mayBeNative(multiplexingArchitectures, m.native) {
		result.Architectures = withoutAny(result.Architectures, multiplexingArchitectures)

		return
	}

	result.Syscalls = settled
}

// loadsWideValue reports whether a profile loads a rule comparing an
// argument against a value or mask above 32 bits, as libseccomp reads the
// condition (see canonicalArg). It looks at the entries without expanding
// them into rules: every condition of an entry that loads any rule ends up
// in one.
func loadsWideValue(profile *specs.LinuxSeccomp) bool {
	def := defaultClause(profile)

	for idx := range profile.Syscalls {
		entry := &profile.Syscalls[idx]
		if !loadsRule(entry, def) {
			continue
		}

		if slices.ContainsFunc(entry.Args, func(arg specs.LinuxSeccompArg) bool {
			return wideArg(canonicalArg(arg))
		}) {
			return true
		}
	}

	return false
}

// withoutAny returns the architectures not in the set, or nil where none
// remain, so that a result covering only the native architecture spells it
// as a merge of two lists without a common architecture does.
func withoutAny(archs []specs.Arch, set map[specs.Arch]bool) []specs.Arch {
	return merge.NilIfEmpty(slices.DeleteFunc(slices.Clone(archs), func(arch specs.Arch) bool {
		return set[arch]
	}))
}

// checkListeners refuses inputs whose listeners a merge cannot reconcile
// (see ErrConflictingListener). The merge takes the listener from the first
// input that sets a listenerPath, which is only sound when every other input
// that names one names the same listener.
//
// Intersect goes further and takes the listener from its first input only.
// Its first input is the baseline the runtime trusts and the others are
// artifacts or pod settings it does not (see Intersect), and a listener is a
// socket on the node: runc hands the notification fd to whatever listens
// there, so an artifact that named a listener and notified on a syscall the
// baseline allows would choose who answers the workload's calls. A later
// input that sets neither listenerPath nor listenerMetadata leaves the
// listener to the baseline; one that sets either must set both exactly as
// the baseline does, which keeps merging a baseline with an earlier result
// of the same merge working. Repeating the path without the metadata is
// refused rather than read as leaving the metadata to the baseline: it
// says something about the listener, and what it says differs.
//
// Inputs are compared by their listener alone, whether or not they notify.
// Where only one of two inputs with different listeners notifies, Union
// could take the listener from that one instead, but which actions reach
// the result depends on the rules of every input, so whether a merge
// succeeds would then change with an unrelated rule. A listener is also a
// node-local setting chosen for a reason the merge cannot see, so the
// conservative reading is that two listeners conflict wherever both are
// named. For Union an input without a listenerPath names no listener, and
// Validate refuses one setting listenerMetadata without it.
func checkListeners(profiles []*specs.LinuxSeccomp, intersect bool) error {
	if intersect {
		return checkBaselineListener(profiles)
	}

	first := -1

	for idx, profile := range profiles {
		if profile.ListenerPath == "" {
			continue
		}

		if first < 0 {
			first = idx

			continue
		}

		err := sameListener(profiles, first, idx)
		if err != nil {
			return err
		}
	}

	return nil
}

// checkBaselineListener is checkListeners for Intersect: every input after
// the first that sets a listenerPath or listenerMetadata must set both as
// the first input does.
func checkBaselineListener(profiles []*specs.LinuxSeccomp) error {
	for idx, profile := range profiles {
		if idx == 0 || (profile.ListenerPath == "" && profile.ListenerMetadata == "") {
			continue
		}

		err := sameListener(profiles, 0, idx)
		if err != nil {
			return err
		}
	}

	return nil
}

// sameListener reports an ErrConflictingListener unless the profiles at
// first and idx set the same listenerPath and listenerMetadata. The error
// is an *InputError naming idx, the later input, which is the one that
// brought the conflict: the listener it conflicts with is named by value.
func sameListener(profiles []*specs.LinuxSeccomp, first, idx int) error {
	chosen, profile := profiles[first], profiles[idx]

	var err error

	switch {
	case profile.ListenerPath != chosen.ListenerPath:
		err = fmt.Errorf(
			"listenerPath: %s: %w (the merge's listener is %s)",
			merge.QuoteBounded(profile.ListenerPath), ErrConflictingListener,
			merge.QuoteBounded(chosen.ListenerPath),
		)
	case profile.ListenerMetadata != chosen.ListenerMetadata:
		err = fmt.Errorf(
			"listenerMetadata: %s: %w (the merge's listener has %s)",
			merge.QuoteBounded(profile.ListenerMetadata), ErrConflictingListener,
			merge.QuoteBounded(chosen.ListenerMetadata),
		)
	default:
		return nil
	}

	return &spm.InputError{Index: idx, Err: err}
}

// resolveListener keeps SCMP_ACT_NOTIFY and the listener together in the
// result, for either strategy. runc refuses to create a container for a
// filter that notifies without a listenerPath, so such a result is refused
// rather than emitted, and rather than rewritten: the action a caller wrote
// is the supervisor it asked for, and both rewriting it and dropping it
// silently lose that.
//
// It holds for every result the two merges can build. The action of an entry
// is always the action of an input entry (see pickClause), Validate requires
// a listenerPath from every input that notifies, and checkListeners refuses
// inputs naming different listeners, so an input notifies only alongside a
// listener the result keeps. The default action is never SCMP_ACT_NOTIFY
// either, since Validate rejects it on every input and the merged default is
// one of theirs. This is the check that keeps that reasoning true.
func resolveListener(profile *specs.LinuxSeccomp) error {
	if profile.ListenerPath != "" {
		return nil
	}

	if !notifiesAny(profile.Syscalls) {
		return nil
	}

	return fmt.Errorf("merge: %w", ErrNotifyWithoutListener)
}

// regroupSyscalls merges entries sharing the same action, errno, and
// argument filters into one multi-name entry, and sorts the result by first
// name, then by argument filter, action, and errno, which is a total order
// over the result. Every entry it is given carries exactly one name, so
// every group has a first name to sort by.
func regroupSyscalls(syscalls []specs.LinuxSyscall) []specs.LinuxSyscall {
	type group struct {
		entry   specs.LinuxSyscall
		argsKey string
		names   map[string]struct{}
	}

	groups := make(map[string]*group)

	for idx := range syscalls {
		entry := &syscalls[idx]

		args := sortedArgs(entry.Args)
		key := resultKey(entry.Action, entry.ErrnoRet, args)

		current, ok := groups[key]
		if !ok {
			current = &group{
				entry: specs.LinuxSyscall{
					Names:    nil,
					Action:   entry.Action,
					ErrnoRet: merge.ClonePtr(entry.ErrnoRet),
					Args:     args,
				},
				argsKey: sortedArgsKey(args),
				names:   make(map[string]struct{}),
			}
			groups[key] = current
		}

		for _, name := range entry.Names {
			current.names[name] = struct{}{}
		}
	}

	ordered := make([]*group, 0, len(groups))

	for _, current := range groups {
		current.entry.Names = slices.Sorted(maps.Keys(current.names))
		ordered = append(ordered, current)
	}

	slices.SortFunc(ordered, func(a, b *group) int {
		return cmp.Or(
			cmp.Compare(a.entry.Names[0], b.entry.Names[0]),
			cmp.Compare(a.argsKey, b.argsKey),
			cmp.Compare(a.entry.Action, b.entry.Action),
			compareUintPtr(a.entry.ErrnoRet, b.entry.ErrnoRet),
		)
	})

	result := make([]specs.LinuxSyscall, 0, len(ordered))
	for _, current := range ordered {
		result = append(result, current.entry)
	}

	return result
}
