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
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"sigs.k8s.io/security-profiles-merger/internal/merge"
)

const (
	maxSyscallArgIndex = 5
	// notifyWriteSyscall is the syscall runc refuses to put behind a
	// notification listener, since the listener answers a request by writing
	// to it.
	notifyWriteSyscall = "write"
	// maxErrno is the largest errno libseccomp accepts for SCMP_ACT_ERRNO,
	// one below the kernel's MAX_ERRNO (4095): libseccomp refuses 4095 as
	// a rule's errno and as the default's, so a runtime fails to load the
	// profile. runc narrows errnoRet to int16, so larger values would wrap
	// into a different errno than the profile author wrote.
	maxErrno = 4094
	// maxTraceMsg is the largest value SCMP_ACT_TRACE carries. It is no
	// errno but a message for the tracer, the 16 bits of SECCOMP_RET_DATA,
	// which libseccomp accepts whole.
	maxTraceMsg = 65535
)

var (
	// ErrUnknownAction is returned by Validate for an unknown action.
	ErrUnknownAction = errors.New("unknown seccomp action")
	// ErrEmptySyscallNames is returned by Validate for an entry without
	// names.
	ErrEmptySyscallNames = errors.New("syscall entry has no names")
	// ErrEmptySyscallName is returned by Validate for an empty syscall name.
	ErrEmptySyscallName = errors.New("empty syscall name")
	// ErrInvalidSyscallName is returned by Validate for a syscall name
	// holding a NUL byte, where a runtime's C API cuts it, and by
	// ValidateArtifact and ValidateStrict for one holding any control
	// character or invalid UTF-8.
	ErrInvalidSyscallName = errors.New("invalid syscall name")
	// ErrDuplicateSyscallName is returned by ValidateStrict for a syscall
	// name in several entries or twice in one entry.
	ErrDuplicateSyscallName = errors.New("duplicate syscall name")
	// ErrUnknownOperator is returned by Validate for an unknown operator.
	ErrUnknownOperator = errors.New("unknown seccomp operator")
	// ErrArgIndexOutOfRange is returned by Validate for an index above 5.
	ErrArgIndexOutOfRange = errors.New("syscall arg index out of range")
	// ErrUnknownArch is returned by Validate for an unknown architecture,
	// and, matching ErrInvalidOptions too, by every method of an Options
	// from ForArch naming one.
	ErrUnknownArch = errors.New("unknown architecture")
	// ErrDuplicateArch is returned by ValidateArtifact and ValidateStrict
	// for a repeated architecture, which Validate and the merges accept.
	ErrDuplicateArch = errors.New("duplicate architecture")
	// ErrUnknownFlag is returned by Validate for an unknown flag.
	ErrUnknownFlag = errors.New("unknown seccomp flag")
	// ErrDuplicateFlag is returned by ValidateArtifact and ValidateStrict
	// for a repeated flag, which Validate and the merges accept.
	ErrDuplicateFlag = errors.New("duplicate seccomp flag")
	// ErrNotifyNotAllowed is returned by ValidateArtifact and ValidateStrict
	// for a profile using SCMP_ACT_NOTIFY.
	ErrNotifyNotAllowed = errors.New("SCMP_ACT_NOTIFY is not allowed")
	// ErrInvalidUTF8Listener is returned by Validate when listenerPath or
	// listenerMetadata is not valid UTF-8, which JSON would carry to the
	// runtime as another value.
	ErrInvalidUTF8Listener = errors.New("listener setting is not valid UTF-8")
	// ErrListenerMetadataWithoutPath is returned by Validate for a
	// listenerMetadata without a listenerPath, which the runtime-spec
	// forbids: the metadata is for the listener, and there is none.
	ErrListenerMetadataWithoutPath = errors.New("set without a listenerPath")
	// ErrListenerNotAllowed is returned by ValidateArtifact and
	// ValidateStrict for a profile setting listenerPath, listenerMetadata or
	// SECCOMP_FILTER_FLAG_WAIT_KILLABLE_RECV.
	ErrListenerNotAllowed = errors.New("listener settings are not allowed")
	// ErrTooManyEntriesPerSyscall is returned by ValidateArtifact and
	// ValidateStrict past MaxArtifactEntriesPerSyscall.
	ErrTooManyEntriesPerSyscall = merge.LimitError("too many entries for syscall")
	// ErrErrnoOutOfRange is returned by ValidateArtifact and ValidateStrict
	// for an errnoRet or defaultErrnoRet libseccomp refuses: 4095 or above
	// on SCMP_ACT_ERRNO, above 65535 on SCMP_ACT_TRACE.
	ErrErrnoOutOfRange = errors.New("errno out of range")
	// ErrUnusedValueTwo is returned by ValidateStrict for valueTwo set with
	// an operator other than SCMP_CMP_MASKED_EQ.
	ErrUnusedValueTwo = errors.New("valueTwo is only used by SCMP_CMP_MASKED_EQ")
	// ErrUnusedErrnoRet is returned by ValidateStrict and ValidateArtifact
	// for errnoRet or defaultErrnoRet on an action other than SCMP_ACT_ERRNO
	// or SCMP_ACT_TRACE, which crun refuses.
	ErrUnusedErrnoRet = errors.New("errnoRet is only used by SCMP_ACT_ERRNO and SCMP_ACT_TRACE")
	// ErrConflictingEntries is returned by ValidateArtifact and
	// ValidateStrict for rules of one syscall, or their copies on a
	// multiplexer, with different results that a runtime refuses or
	// evaluates in its own order, and for conditional multiplexer rules next
	// to rules of a syscall it carries. See the Conflicting rules section.
	ErrConflictingEntries = errors.New("conflicting entries")
	// ErrTooManyRulesPerSyscall is returned by ValidateArtifact and
	// ValidateStrict past MaxArtifactRulesPerSyscall.
	ErrTooManyRulesPerSyscall = merge.LimitError("too many rules for syscall")
	// ErrTooManyNamesPerEntry is returned by ValidateArtifact and
	// ValidateStrict past MaxArtifactNamesPerEntry.
	ErrTooManyNamesPerEntry = merge.LimitError("too many syscall names in entry")
	// ErrTooManyRules is returned by ValidateArtifact and ValidateStrict
	// past MaxArtifactRules.
	ErrTooManyRules = merge.LimitError("too many rules in profile")
	// ErrTooManyEntries is returned by ValidateArtifact and ValidateStrict
	// past MaxArtifactEntries.
	ErrTooManyEntries = merge.LimitError("too many syscall entries in profile")
	// ErrValueTooWide is returned by ValidateArtifact and ValidateStrict for
	// a condition whose value or mask (valueTwo only within the mask of a
	// SCMP_CMP_MASKED_EQ) is above 32 bits while the filter covers a 32-bit
	// architecture, listed or native, in an entry that loads a rule: one
	// equal to the default loads none.
	ErrValueTooWide = errors.New("argument value above 32 bits on a 32-bit architecture")
	// ErrMixedByteOrder is returned by ValidateArtifact and ValidateStrict,
	// and by Intersect in an InputError, for an architecture of another byte
	// order than the native one, or, where that is not known, a list mixing
	// both.
	ErrMixedByteOrder = errors.New("architecture of another byte order")
	// ErrNotifyUnsupported is returned by Validate for SCMP_ACT_NOTIFY as
	// the default action or on the write syscall, which runc refuses.
	ErrNotifyUnsupported = errors.New("SCMP_ACT_NOTIFY is not supported here")
)

// MaxArtifactEntriesPerSyscall bounds the entries naming one syscall in a
// profile ValidateArtifact accepts.
const MaxArtifactEntriesPerSyscall = 128

// MaxArtifactNamesPerEntry bounds the syscall names of one entry in a
// profile ValidateArtifact accepts.
const MaxArtifactNamesPerEntry = 1024

// MaxArtifactRules bounds the rules a profile ValidateArtifact accepts
// loads in total, counted as for MaxArtifactRulesPerSyscall.
const MaxArtifactRules = 16384

// MaxArtifactEntries bounds the syscall entries of a profile
// ValidateArtifact accepts. Only entries loading no rule, such as one equal
// to the default, can exceed it without exceeding MaxArtifactRules; the two
// may change separately.
const MaxArtifactEntries = 16384

// MaxArtifactRulesPerSyscall bounds the rules one syscall loads in a
// profile ValidateArtifact accepts, counted as runtimes add them (see the
// Cost bounds section). Intersect merges an artifact within it precisely
// against a baseline with up to a dozen filtered entries for the syscall.
const MaxArtifactRulesPerSyscall = 256

// Validate checks what a runtime needs to load a profile at all: known
// actions, operators, architectures and flags; non-empty syscall names
// without a NUL byte ([ErrInvalidSyscallName]); argument indices 0 to 5;
// SCMP_ACT_NOTIFY only where runc loads it ([ErrNotifyUnsupported]) and
// with a listenerPath ([ErrNotifyWithoutListener]); listener settings in
// valid UTF-8 ([ErrInvalidUTF8Listener]); no listenerMetadata without a
// listenerPath ([ErrListenerMetadataWithoutPath]); and no field of the
// runtime-spec types this package does not read
// ([ErrUnsupportedSpecField]). Intersect and Union run it on every input.
//
// It enforces none of the MaxArtifact limits and does not range-check errno
// values (see the Errno values section). Failures are collected into one
// bounded report, which matches [ErrMoreProblems] when it left some out (see
// Reading a validation error in the spm package documentation).
func Validate(profile *specs.LinuxSeccomp) error {
	if profile == nil {
		return ErrNilProfile
	}

	var errs merge.Collector

	if !isKnownAction(profile.DefaultAction) {
		errs.Report(func() error {
			return unknownActionError(profile.DefaultAction, "defaultAction")
		})
	}

	// runc refuses SCMP_ACT_NOTIFY as the default action ("SCMP_ACT_NOTIFY
	// cannot be used as default action"); see validateEntry for the rest.
	if profile.DefaultAction == specs.ActNotify {
		errs.Reportf("defaultAction: %w", ErrNotifyUnsupported)
	}

	validateArchitecturesAndFlags(&errs, profile)
	validateListenerEncoding(&errs, profile)

	if profile.ListenerMetadata != "" && profile.ListenerPath == "" {
		errs.Reportf("listenerMetadata: %w", ErrListenerMetadataWithoutPath)
	}

	reportUnreadProfileFields(&errs, profile)

	notifies := false

	// Each entry is checked whole before the next, so the report names the
	// problems in the order the profile holds them.
	for idx := range profile.Syscalls {
		entry := &profile.Syscalls[idx]
		notifies = notifies || entry.Action == specs.ActNotify

		validateEntry(&errs, idx, entry)
	}

	// Reported once for the profile: the listener is a profile-wide setting,
	// so naming every entry that notifies would repeat one problem.
	if notifies && profile.ListenerPath == "" {
		errs.Reportf("listenerPath: %w", ErrNotifyWithoutListener)
	}

	return errs.Err()
}

// validateListenerEncoding reports a listenerPath or listenerMetadata that
// is not valid UTF-8 (see ErrInvalidUTF8Listener).
func validateListenerEncoding(errs *merge.Collector, profile *specs.LinuxSeccomp) {
	for _, field := range []struct{ name, value string }{
		{"listenerPath", profile.ListenerPath},
		{"listenerMetadata", profile.ListenerMetadata},
	} {
		if !utf8.ValidString(field.value) {
			errs.Reportf(
				"%s: %s: %w", field.name, merge.QuoteBounded(field.value), ErrInvalidUTF8Listener,
			)
		}
	}
}

// validateArchitecturesAndFlags reports the architectures and flags this
// package does not know.
func validateArchitecturesAndFlags(errs *merge.Collector, profile *specs.LinuxSeccomp) {
	for idx, arch := range profile.Architectures {
		if !isKnownArch(arch) {
			errs.Report(func() error {
				return fmt.Errorf(
					"architectures[%d]: %s: %w",
					idx,
					merge.QuoteBounded(string(arch)),
					ErrUnknownArch,
				)
			})
		}
	}

	for idx, flag := range profile.Flags {
		if !isKnownFlag(flag) {
			errs.Report(func() error {
				return fmt.Errorf(
					"flags[%d]: %s: %w", idx, merge.QuoteBounded(string(flag)), ErrUnknownFlag,
				)
			})
		}
	}
}

// validateEntry reports what Validate rejects in the syscall entry at idx:
// no names, an empty name or one holding a NUL, an unknown action, an
// unknown operator or an argument index out of range, and SCMP_ACT_NOTIFY
// where runc refuses to load it.
//
// runc refuses SCMP_ACT_NOTIFY on the write syscall, which the listener
// needs to answer a notification, and without a listenerPath to hand the
// notification to, which Validate reports once for the profile. libseccomp
// itself accepts both, so these are runc constraints, and a profile that
// hits one fails container creation rather than producing a weaker filter.
//
// The listener is required here rather than left to the merge because a
// profile that notifies into nothing is not loadable on its own, and the
// merge would otherwise have to choose between rewriting the action, which
// silently drops the caller's supervisor, and refusing a profile Validate
// accepted. With the requirement, the listener travels with every input that
// notifies, and the merge takes it from the first input that sets one, so no
// result can notify without one (see resolveListener).
func validateEntry(errs *merge.Collector, idx int, entry *specs.LinuxSyscall) {
	if len(entry.Names) == 0 {
		errs.Report(func() error {
			return fmt.Errorf("syscalls[%d].names: %w", idx, ErrEmptySyscallNames)
		})
	}

	for nameIdx, name := range entry.Names {
		switch {
		case name == "":
			errs.Report(func() error {
				return fmt.Errorf("syscalls[%d].names[%d]: %w", idx, nameIdx, ErrEmptySyscallName)
			})
		case hasNUL(name):
			errs.Report(func() error {
				return invalidNameError(idx, nameIdx, name)
			})
		}
	}

	if !isKnownAction(entry.Action) {
		errs.Report(func() error {
			return unknownActionError(entry.Action, fmt.Sprintf("syscalls[%d].action", idx))
		})
	}

	if entry.Action == specs.ActNotify && slices.Contains(entry.Names, notifyWriteSyscall) {
		errs.Report(func() error {
			return fmt.Errorf(
				"syscalls[%d].action: %w (on %s)", idx, ErrNotifyUnsupported, notifyWriteSyscall,
			)
		})
	}

	validateArgs(errs, idx, entry.Args)
	reportUnreadEntryFields(errs, idx, entry)
}

// validateArgs reports the unknown operators and the argument indices out of
// range among the arguments of the entry at idx.
func validateArgs(errs *merge.Collector, idx int, args []specs.LinuxSeccompArg) {
	for argIdx, arg := range args {
		if !isKnownOperator(arg.Op) {
			errs.Report(func() error {
				return fmt.Errorf(
					"syscalls[%d].args[%d].op: %s: %w",
					idx, argIdx, merge.QuoteBounded(string(arg.Op)), ErrUnknownOperator,
				)
			})
		}

		if arg.Index > maxSyscallArgIndex {
			errs.Report(func() error {
				return fmt.Errorf(
					"syscalls[%d].args[%d].index: %d: %w (max %d)",
					idx, argIdx, arg.Index, ErrArgIndexOutOfRange, maxSyscallArgIndex,
				)
			})
		}
	}
}

// invalidNameError reports the syscall name at nameIdx of the entry at idx
// as invalid.
func invalidNameError(idx, nameIdx int, name string) error {
	return fmt.Errorf(
		"syscalls[%d].names[%d]: %s: %w",
		idx,
		nameIdx,
		merge.QuoteBounded(name),
		ErrInvalidSyscallName,
	)
}

// ValidateStrict rejects everything ValidateArtifact rejects, and also
// duplicate syscall names, once per name several entries use and once per
// entry repeating a name ([ErrDuplicateSyscallName]), and valueTwo on an
// operator that ignores it ([ErrUnusedValueTwo]). A profile it accepts
// passes ValidateArtifact and Validate.
//
// It is a lint for profiles a person writes. The runtime-spec allows one
// syscall in several entries, as runtime defaults and merge results use it,
// and a baseline that notifies fails here, so check those with Validate. It
// depends on the native architecture as ValidateArtifact does
// ([Options.ValidateStrict]).
func ValidateStrict(profile *specs.LinuxSeccomp) error {
	return ForHost().ValidateStrict(profile)
}

// strictChecks returns the checks ValidateStrict runs besides Validate, for
// a node with the given native architecture: those of ValidateArtifact and
// its own.
func strictChecks(native specs.Arch) []profileCheck {
	return append(artifactChecks(native), validateDuplicateSyscallNames, validateUnusedValueTwo)
}

// ValidateArtifact checks an artifact, a profile from an untrusted source
// such as an OCI artifact a container runtime pulls (KEP-6061), so that it
// loads on every runtime and merges precisely. On top of Validate it
// rejects:
//
//   - duplicate architectures and flags (ErrDuplicateArch,
//     ErrDuplicateFlag);
//   - errno values libseccomp refuses (ErrErrnoOutOfRange) and an errno on
//     an action that ignores it, which crun refuses (ErrUnusedErrnoRet);
//   - SCMP_ACT_NOTIFY and the listener settings, which belong to the node
//     (ErrNotifyNotAllowed, ErrListenerNotAllowed);
//   - syscall names holding a control character or not valid UTF-8
//     (ErrInvalidSyscallName);
//   - an architecture of another byte order than the native one
//     (ErrMixedByteOrder), and a value above 32 bits where the filter
//     covers a 32-bit architecture (ErrValueTooWide);
//   - a profile past a MaxArtifact limit (ErrTooManyEntries,
//     ErrTooManyNamesPerEntry, ErrTooManyRules, checked first and on their
//     own, and ErrTooManyEntriesPerSyscall, ErrTooManyRulesPerSyscall,
//     checked next and reported ahead of every other failure, so that a
//     profile past one always matches its error); each matches
//     ErrArtifactTooLarge;
//   - rules of one syscall with different results that a runtime refuses
//     or evaluates in its own order (ErrConflictingEntries);
//   - a filter in which libseccomp loses the rules of a covered
//     architecture as written (ErrLostRules), which the merges may settle
//     and accept.
//
// It allows duplicate syscall names and ignores valueTwo where runtimes
// ignore it. Failures are bounded as for Validate. A profile that passes may
// still hold rules libseccomp evaluates in its own order, so a runtime loads
// its intersection with the baseline rather than the artifact itself. What
// it reports depends on the native architecture
// ([Options.ValidateArtifact]). See the Conflicting rules, Cost bounds and
// Architectures sections.
func ValidateArtifact(profile *specs.LinuxSeccomp) error {
	return ForHost().ValidateArtifact(profile)
}

// artifactChecks returns the checks ValidateArtifact runs besides Validate,
// for a node with the given native architecture. ValidateStrict runs them
// too, so that a profile it accepts is accepted by ValidateArtifact as well.
func artifactChecks(native specs.Arch) []profileCheck {
	return []profileCheck{
		validateWellFormed,
		validateSyscallNameSpelling,
		validateNoNotify,
		validateNoListener,
		validateUnusedErrnoRet,
		func(errs *merge.Collector, profile *specs.LinuxSeccomp) {
			validateValueWidth(errs, profile, native)
		},
		func(errs *merge.Collector, profile *specs.LinuxSeccomp) {
			validateByteOrder(errs, profile, native)
		},
	}
}

// validateByteOrder reports the listed architectures whose byte order
// differs from that of native, each once, at its first position, or where
// native is not known, a list holding architectures of both byte orders,
// once for the list, naming those of each (see ErrMixedByteOrder). Validate
// reports the architectures this package does not know, whose byte order
// it cannot tell.
func validateByteOrder(errs *merge.Collector, profile *specs.LinuxSeccomp, native specs.Arch) {
	listed := make(map[byteOrder][]specs.Arch)
	seen := make(map[specs.Arch]bool)

	for idx, arch := range profile.Architectures {
		order := byteOrders[arch]
		if order == 0 || seen[arch] {
			continue
		}

		seen[arch] = true
		listed[order] = append(listed[order], arch)

		if native != "" && order != byteOrders[native] {
			errs.Report(func() error {
				return fmt.Errorf(
					"architectures[%d]: %s: %w (%s, native %s is %s)",
					idx, merge.QuoteBounded(string(arch)), ErrMixedByteOrder,
					order, native, byteOrders[native],
				)
			})
		}
	}

	if native == "" && len(listed[bigEndian]) > 0 && len(listed[littleEndian]) > 0 {
		errs.Reportf(
			"architectures: %w (%s %s next to %s %s)",
			ErrMixedByteOrder,
			bigEndian, quoteArchs(listed[bigEndian]),
			littleEndian, quoteArchs(listed[littleEndian]),
		)
	}
}

// quoteArchs renders architectures as a comma-separated list of quoted
// names. There are no more of them than this package knows.
func quoteArchs(archs []specs.Arch) string {
	quoted := make([]string, len(archs))
	for idx, arch := range archs {
		quoted[idx] = merge.QuoteBounded(string(arch))
	}

	return strings.Join(quoted, ", ")
}

// validateValueWidth reports conditions against a value above 32 bits when
// the filter covers a 32-bit architecture (see ErrValueTooWide). Values are
// read as libseccomp reads them, so a valueTwo it ignores or masks away does
// not count, and so is an entry that loads no rule, such as one equal to
// the default, which runtimes skip, as loadsWideValue skips it. native is
// the architecture the runtime adds to the filter.
func validateValueWidth(errs *merge.Collector, profile *specs.LinuxSeccomp, native specs.Arch) {
	if !coversAny(profile.Architectures, narrowArchitectures, native) {
		return
	}

	def := defaultClause(profile)

	for idx := range profile.Syscalls {
		if !loadsRule(&profile.Syscalls[idx], def) {
			continue
		}

		for argIdx, arg := range profile.Syscalls[idx].Args {
			if wideArg(canonicalArg(arg)) {
				errs.Report(func() error {
					return fmt.Errorf(
						"syscalls[%d].args[%d]: %w (value %#x, valueTwo %#x)",
						idx, argIdx, ErrValueTooWide, arg.Value, arg.ValueTwo,
					)
				})
			}
		}
	}
}

// profileCheck reports what it refuses in a profile to errs.
type profileCheck func(errs *merge.Collector, profile *specs.LinuxSeccomp)

// validateWith runs validateProfileSize, and where the profile is within its
// bounds the per-syscall limits, Validate and every check, collecting all
// failures into one error. The per-syscall limits come first, so that the
// bound on the report does not leave them all out and a profile past one
// always matches ErrArtifactTooLarge. The rule checks for a node with the
// given native architecture (validateSyscallRules and validateLostRules)
// run only when Validate passes: they expand entries the way a runtime
// loads them, which assumes known operators and argument indices in range,
// so that an entry without a repeated index carries at most one condition
// per index.
func validateWith(profile *specs.LinuxSeccomp, native specs.Arch, checks []profileCheck) error {
	if profile == nil {
		return ErrNilProfile
	}

	err := validateProfileSize(profile)
	if err != nil {
		return err
	}

	// The names of an alias libseccomp reads as one syscall here count as
	// one syscall for the per-syscall limits and the rule checks, and an
	// entry naming both, as a merge result does, adds its rules to that
	// syscall once (see dropAliasRepeats).
	aliases := collapsedAliases(profile.Architectures, native)
	counted := *profile
	counted.Syscalls = dropAliasRepeats(profile.Syscalls, aliases)

	// The profile loads at most MaxArtifactRules rules, so the counts hold
	// at most that many names.
	counts := clauseCounts(counted.Syscalls, defaultClause(profile))

	var errs merge.Collector

	labels := aliasLabels(profile.Syscalls, aliases)

	validateEntryCount(&errs, &counted, aliases, labels)
	validateRuleCount(&errs, counts, aliases, labels)

	err = Validate(profile)
	errs.Add(err)

	for _, check := range checks {
		check(&errs, profile)
	}

	if err == nil {
		validateSyscallRules(&errs, &counted, native, aliases, counts)
		errs.Add(validateLostRules(profile, native))
	}

	return errs.Err()
}

// validateWellFormed runs the checks shared by ValidateStrict and
// ValidateArtifact that do not depend on trust: duplicate architectures and
// flags, and out-of-range errno values.
func validateWellFormed(errs *merge.Collector, profile *specs.LinuxSeccomp) {
	reportDuplicates(errs, "architectures", profile.Architectures, ErrDuplicateArch)
	reportDuplicates(errs, "flags", profile.Flags, ErrDuplicateFlag)
	validateErrnoRange(errs, profile)
}

// validateErrnoRange checks errno values where a runtime reads them; values
// on other actions are reported through validateUnusedErrnoRet instead.
func validateErrnoRange(errs *merge.Collector, profile *specs.LinuxSeccomp) {
	ret := profile.DefaultErrnoRet
	if limit, ok := errnoLimit(profile.DefaultAction); ok && ret != nil && *ret > limit {
		errs.Reportf(
			"defaultErrnoRet: %d: %w (max %d for %s)",
			*ret,
			ErrErrnoOutOfRange,
			limit,
			profile.DefaultAction,
		)
	}

	for idx := range profile.Syscalls {
		entry := &profile.Syscalls[idx]

		ret := entry.ErrnoRet
		if limit, ok := errnoLimit(entry.Action); ok && ret != nil && *ret > limit {
			errs.Report(func() error {
				return fmt.Errorf(
					"syscalls[%d].errnoRet: %d: %w (max %d for %s)",
					idx,
					*ret,
					ErrErrnoOutOfRange,
					limit,
					entry.Action,
				)
			})
		}
	}
}

// errnoLimit returns the largest errnoRet libseccomp accepts for action,
// and whether the action reads errnoRet at all (see errnoSignificant): an
// errno below MAX_ERRNO for SCMP_ACT_ERRNO, and a 16-bit message for
// SCMP_ACT_TRACE.
func errnoLimit(action specs.LinuxSeccompAction) (uint, bool) {
	if !errnoSignificant(action) {
		return 0, false
	}

	if action == specs.ActTrace {
		return maxTraceMsg, true
	}

	return maxErrno, true
}

func validateUnusedValueTwo(errs *merge.Collector, profile *specs.LinuxSeccomp) {
	for idx := range profile.Syscalls {
		for argIdx, arg := range profile.Syscalls[idx].Args {
			if arg.ValueTwo != 0 && arg.Op != specs.OpMaskedEqual {
				errs.Report(func() error {
					return fmt.Errorf(
						"syscalls[%d].args[%d].valueTwo: %d: %w (op %s)",
						idx,
						argIdx,
						arg.ValueTwo,
						ErrUnusedValueTwo,
						merge.QuoteBounded(string(arg.Op)),
					)
				})
			}
		}
	}
}

func validateUnusedErrnoRet(errs *merge.Collector, profile *specs.LinuxSeccomp) {
	if profile.DefaultErrnoRet != nil && !errnoSignificant(profile.DefaultAction) {
		errs.Reportf(
			"defaultErrnoRet: %d: %w (defaultAction %s)",
			*profile.DefaultErrnoRet, ErrUnusedErrnoRet,
			merge.QuoteBounded(string(profile.DefaultAction)),
		)
	}

	for idx := range profile.Syscalls {
		entry := &profile.Syscalls[idx]
		if entry.ErrnoRet != nil && !errnoSignificant(entry.Action) {
			errs.Report(func() error {
				return fmt.Errorf(
					"syscalls[%d].errnoRet: %d: %w (action %s)",
					idx,
					*entry.ErrnoRet,
					ErrUnusedErrnoRet,
					merge.QuoteBounded(string(entry.Action)),
				)
			})
		}
	}
}

// validateProfileSize bounds the profile as a whole: the entries it may
// hold, the names one entry may carry and the rules the profile may load.
// The per-syscall caps count per name, so neither of them bounds an entry
// that names a hundred thousand syscalls, and the rules such a profile loads
// are the product of its name and condition counts (see MaxArtifactRules).
// It counts the rules without expanding the entries, so it costs one pass
// over the profile and allocates only for what it reports, and none for a
// profile past MaxArtifactEntries, which it reports without reading.
func validateProfileSize(profile *specs.LinuxSeccomp) error {
	if entries := len(profile.Syscalls); entries > MaxArtifactEntries {
		return fmt.Errorf(
			"syscalls: %d entries: %w (max %d)",
			entries,
			ErrTooManyEntries,
			MaxArtifactEntries,
		)
	}

	var errs merge.Collector

	for idx := range profile.Syscalls {
		names := len(profile.Syscalls[idx].Names)
		if names > MaxArtifactNamesPerEntry {
			errs.Report(func() error {
				return fmt.Errorf(
					"syscalls[%d].names: %d names: %w (max %d)",
					idx, names, ErrTooManyNamesPerEntry, MaxArtifactNamesPerEntry,
				)
			})
		}
	}

	total := totalClauses(profile.Syscalls, defaultClause(profile))
	if total > MaxArtifactRules {
		errs.Reportf("syscalls: %d rules: %w (max %d)", total, ErrTooManyRules, MaxArtifactRules)
	}

	return errs.Err()
}

// validateSyscallNameSpelling reports a syscall name holding a control
// character, which no syscall name holds and which forges a line wherever
// the profile is rendered, or one that is not valid UTF-8, which no syscall
// name is either and which JSON cannot carry: written out, it reads as
// another name. A name holding a NUL is left to Validate, which reports it
// for every profile.
func validateSyscallNameSpelling(errs *merge.Collector, profile *specs.LinuxSeccomp) {
	for idx := range profile.Syscalls {
		for nameIdx, name := range profile.Syscalls[idx].Names {
			if (hasControlByte(name) || !utf8.ValidString(name)) && !hasNUL(name) {
				errs.Report(func() error { return invalidNameError(idx, nameIdx, name) })
			}
		}
	}
}

// hasNUL reports whether a name holds a NUL, which ends the name in the C
// API a runtime resolves it through, so that the runtime reads another name
// than this package does.
func hasNUL(name string) bool {
	return strings.IndexByte(name, 0) >= 0
}

// hasControlByte reports whether a name holds a C0 control character or DEL.
func hasControlByte(name string) bool {
	for idx := range len(name) {
		if name[idx] < 0x20 || name[idx] == 0x7f {
			return true
		}
	}

	return false
}

func validateNoNotify(errs *merge.Collector, profile *specs.LinuxSeccomp) {
	if profile.DefaultAction == specs.ActNotify {
		errs.Reportf("defaultAction: %w", ErrNotifyNotAllowed)
	}

	for idx := range profile.Syscalls {
		if profile.Syscalls[idx].Action == specs.ActNotify {
			errs.Report(func() error {
				return fmt.Errorf("syscalls[%d].action: %w", idx, ErrNotifyNotAllowed)
			})
		}
	}
}

// validateEntryCount reports every syscall named in more entries than
// MaxArtifactEntriesPerSyscall, in name order, counting the two names of
// each of the aliases given as one syscall, which labels name (see
// aliasLabels).
func validateEntryCount(
	errs *merge.Collector, profile *specs.LinuxSeccomp, aliases []syscallAlias,
	labels countLabels,
) {
	counts := make(map[string]int)

	for idx := range profile.Syscalls {
		for _, name := range profile.Syscalls[idx].Names {
			counts[name]++
		}
	}

	counts = joinAliasCounts(counts, aliases)

	for _, name := range slices.Sorted(maps.Keys(counts)) {
		if counts[name] > MaxArtifactEntriesPerSyscall {
			errs.Report(func() error {
				return fmt.Errorf(
					"syscalls: %s in %d entries: %w (max %d)",
					labels.of(name), counts[name], ErrTooManyEntriesPerSyscall,
					MaxArtifactEntriesPerSyscall,
				)
			})
		}
	}
}

func validateNoListener(errs *merge.Collector, profile *specs.LinuxSeccomp) {
	if profile.ListenerPath != "" {
		errs.Reportf("listenerPath: %w", ErrListenerNotAllowed)
	}

	// Validate refuses a listenerMetadata without a listenerPath already
	// (ErrListenerMetadataWithoutPath), so it is reported once.
	if profile.ListenerMetadata != "" && profile.ListenerPath != "" {
		errs.Reportf("listenerMetadata: %w", ErrListenerNotAllowed)
	}

	if idx := slices.Index(profile.Flags, specs.LinuxSeccompFlagWaitKillableRecv); idx >= 0 {
		errs.Reportf(
			"flags[%d]: %s: %w",
			idx, merge.QuoteBounded(string(specs.LinuxSeccompFlagWaitKillableRecv)),
			ErrListenerNotAllowed,
		)
	}
}

// validateDuplicateSyscallNames reports each syscall name several entries
// name once, at the second of them and naming the first, and separately at
// each entry that repeats a name within itself. One report per name, rather
// than one per entry naming it, keeps a name an artifact repeats in every
// entry from crowding the other problems out of the bounded report. The
// reports come in the order of the entries and of the names within each.
func validateDuplicateSyscallNames(errs *merge.Collector, profile *specs.LinuxSeccomp) {
	// The entry that names each name first, or -1 once the name has been
	// reported as named by several entries.
	first := make(map[string]int)

	for idx, sc := range profile.Syscalls {
		// Each name of the entry, mapped to whether it was reported as
		// repeated within the entry.
		inEntry := make(map[string]bool, len(sc.Names))

		for _, name := range sc.Names {
			reported, again := inEntry[name]

			switch {
			case again && !reported:
				inEntry[name] = true

				errs.Report(func() error {
					return fmt.Errorf("syscalls[%d]: %s: %w (repeated within the entry)",
						idx, merge.QuoteBounded(name), ErrDuplicateSyscallName)
				})
			case again:
			default:
				inEntry[name] = false

				earlier, seen := first[name]

				switch {
				case !seen:
					first[name] = idx

					continue
				case earlier < 0:
					continue
				}

				first[name] = -1

				errs.Report(func() error {
					return fmt.Errorf("syscalls[%d]: %s: %w (first named by entry %d)",
						idx, merge.QuoteBounded(name), ErrDuplicateSyscallName, earlier)
				})
			}
		}
	}
}

// isKnownAction reports whether this package knows the action.
func isKnownAction(action specs.LinuxSeccompAction) bool {
	return restrictiveness(action) != levelUnknown
}

// unknownActionError names the offending action, which an artifact chooses
// freely and at any length, so the value is bounded before it reaches an
// error a runtime may log. The caller builds it only for an action
// isKnownAction refuses, and only while the report has room for it.
func unknownActionError(action specs.LinuxSeccompAction, context string) error {
	return fmt.Errorf("%s: %s: %w", context, merge.QuoteBounded(string(action)), ErrUnknownAction)
}

func isKnownOperator(op specs.LinuxSeccompOperator) bool {
	switch op {
	case specs.OpNotEqual, specs.OpLessThan, specs.OpLessEqual,
		specs.OpEqualTo, specs.OpGreaterEqual, specs.OpGreaterThan,
		specs.OpMaskedEqual:
		return true
	default:
		return false
	}
}

// isKnownArch reports whether this package knows the architecture: whether
// it is one of syscallArchitectures, which are the ones the runtime-spec
// names.
func isKnownArch(arch specs.Arch) bool {
	return archBit(arch) != 0
}

func isKnownFlag(flag specs.LinuxSeccompFlag) bool {
	switch flag {
	case specs.LinuxSeccompFlagLog,
		specs.LinuxSeccompFlagSpecAllow,
		specs.LinuxSeccompFlagWaitKillableRecv,
		flagTsync:
		return true
	default:
		return false
	}
}

// reportDuplicates reports every value of the list field that repeats an
// earlier one, matching sentinel.
func reportDuplicates[T ~string](errs *merge.Collector, field string, values []T, sentinel error) {
	merge.ReportDuplicateValues(errs, field, values,
		func(idx int) T { return values[idx] }, sentinel)
}
