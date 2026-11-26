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
	"fmt"
	"slices"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"sigs.k8s.io/security-profiles-merger/internal/merge"
	"sigs.k8s.io/security-profiles-merger/internal/strictjson"
	"sigs.k8s.io/security-profiles-merger/spm"
)

// ErrInvalidOptions is [spm.ErrInvalidOptions], matched by every error a
// method returns for an Options naming no node it can check for: the zero
// value, or a ForArch architecture this package does not know, which also
// matches ErrUnknownArch. It comes before any profile is read and never in
// an InputError, so it never matches what an artifact listing an unknown
// architecture returns.
var ErrInvalidOptions = spm.ErrInvalidOptions

// Options names the node a merge, diff or check is for. Runtimes always add
// the node's native architecture to a filter, and the 32-bit and
// multiplexing architectures are read differently where one of them is
// native (see the Architectures section), so the results depend on it.
//
//   - ForArch names the native architecture of another node.
//   - ForHost names the node this program runs on: the native architecture
//     of the running program (see NativeArchitecture), or, on a platform
//     without one, any architecture as for ForAnyArch. The plain functions
//     of this package are the methods of ForHost().
//   - ForAnyArch names a node whose native architecture is not known. The
//     merges and validators then settle and check for every architecture
//     this package knows, crun's C-only natives included, so a result loads
//     safely wherever it runs, at the cost of rules a known 64-bit node
//     keeps. Only a list mixing byte orders is refused by Intersect and the
//     stricter validators ([ErrMixedByteOrder]), and a narrow denylist may
//     be refused with [ErrLostRules] for an architecture no Go program runs
//     on, as one denying newfstatat outright and mmap on a condition is for
//     parisc, and an x86_64 one for x32. Diff then implies no architecture
//     and compares the lists as written.
//
// The zero value names no node, and every method that returns an error
// reports it with [ErrInvalidOptions]. Options values are comparable, and
// equal when they come from the same constructor with the same argument:
// ForHost() and ForArch of the running program's architecture behave alike
// but are not equal. Validate does not depend on the node and is a method
// so that every check can run through an Options; UnmarshalStrict,
// Marshal, Clone, FormatProfile and FormatSyscall have no method.
type Options struct {
	node nodeKind
	arch specs.Arch
}

// nodeKind is how an Options names its node. The zero value names none.
type nodeKind int

const (
	nodeNone nodeKind = iota
	nodeArch
	nodeHost
	nodeAnyArch
)

// ForArch returns the Options of a node whose native architecture is arch.
// Every method that returns an error reports an architecture this package
// does not know, the empty one included, with an error matching both
// ErrInvalidOptions and ErrUnknownArch, before it looks at a profile.
func ForArch(arch specs.Arch) Options {
	return Options{node: nodeArch, arch: arch}
}

// ForHost returns the Options of the node this program runs on, which the
// plain functions of this package assume.
func ForHost() Options {
	return Options{node: nodeHost}
}

// ForAnyArch returns the Options of a node whose native architecture is not
// known (see Options).
func ForAnyArch() Options {
	return Options{node: nodeAnyArch}
}

// Intersect is Intersect for the node the options name.
func (o Options) Intersect(profiles ...*specs.LinuxSeccomp) (*specs.LinuxSeccomp, error) {
	native, err := o.native()
	if err != nil {
		return nil, err
	}

	return foldProfiles(profiles, intersectRulesFor(native))
}

// Union is Union for the node the options name.
func (o Options) Union(profiles ...*specs.LinuxSeccomp) (*specs.LinuxSeccomp, error) {
	native, err := o.native()
	if err != nil {
		return nil, err
	}

	return foldProfiles(profiles, unionRulesFor(native))
}

// Diff is Diff for the node the options name: its native architecture is
// implied on both sides and never reported as a difference. With ForAnyArch
// the architecture lists are compared as written.
func (o Options) Diff(left, right *specs.LinuxSeccomp) (*ProfileDiff, error) {
	native, err := o.native()
	if err != nil {
		return nil, err
	}

	return diffProfiles(native, left, right)
}

// Validate is Validate, once the options name a node. What it checks does
// not depend on the node.
func (o Options) Validate(profile *specs.LinuxSeccomp) error {
	_, err := o.native()
	if err != nil {
		return err
	}

	return Validate(profile)
}

// ValidateArtifact is ValidateArtifact for the node the options name, whose
// native architecture decides ErrValueTooWide, the multiplexer conflicts of
// ErrConflictingEntries, ErrMixedByteOrder and ErrLostRules.
func (o Options) ValidateArtifact(profile *specs.LinuxSeccomp) error {
	native, err := o.native()
	if err != nil {
		return err
	}

	return validateWith(profile, native, artifactChecks(native))
}

// ValidateStrict is ValidateStrict for the node the options name. It accepts
// only what ValidateArtifact of the same options accepts.
func (o Options) ValidateStrict(profile *specs.LinuxSeccomp) error {
	native, err := o.native()
	if err != nil {
		return err
	}

	return validateWith(profile, native, strictChecks(native))
}

// ValidateBaseline is ValidateBaseline for the node the options name.
func (o Options) ValidateBaseline(baseline *specs.LinuxSeccomp) error {
	err := o.Validate(baseline)
	if err != nil {
		return err
	}

	settled, err := o.Intersect(baseline)
	if err != nil {
		return merge.SoleInputError(err)
	}

	diff, err := o.Diff(baseline, settled)
	if err != nil {
		return err
	}

	if !diff.Equal {
		return fmt.Errorf("%w: %s", ErrInexactBaseline, diff)
	}

	// A merge with any artifact reads the baseline next to another input,
	// which can settle what the baseline alone keeps: a value above 32 bits
	// is read as a 32-bit architecture compares it once another input takes
	// part, so the merge drops the 32-bit architectures or collapses the
	// syscall. An artifact allowing everything on the same architectures,
	// with the same flags and listener, takes nothing away of its own, so
	// what its intersection with the baseline lacks, every merge lacks.
	merged, err := o.Intersect(baseline, allowingAll(baseline))
	if err != nil {
		return merge.SoleInputError(err)
	}

	diff, err = o.Diff(baseline, merged)
	if err != nil {
		return err
	}

	if !diff.Equal {
		return fmt.Errorf("%w: next to an artifact: %s", ErrInexactBaseline, diff)
	}

	return nil
}

// allowingAll returns the profile that allows every call on the
// architectures of the baseline, with its flags and listener, so that an
// intersection with it settles only what the baseline brings.
func allowingAll(baseline *specs.LinuxSeccomp) *specs.LinuxSeccomp {
	return &specs.LinuxSeccomp{
		DefaultAction:    specs.ActAllow,
		DefaultErrnoRet:  nil,
		Architectures:    slices.Clone(baseline.Architectures),
		Flags:            slices.Clone(baseline.Flags),
		ListenerPath:     baseline.ListenerPath,
		ListenerMetadata: baseline.ListenerMetadata,
		Syscalls:         nil,
	}
}

// DecodeArtifact is DecodeArtifact for the node the options name.
func (o Options) DecodeArtifact(data []byte) (*specs.LinuxSeccomp, error) {
	_, err := o.native()
	if err != nil {
		return nil, err
	}

	return strictjson.DecodeValidated(data, UnmarshalStrict, o.ValidateArtifact)
}

// String names the constructor the options come from, as Go source spells
// the call, such as `ForArch("SCMP_ARCH_X86_64")`, or "Options{}" for the
// zero value. The architecture is quoted and bounded in length.
func (o Options) String() string {
	switch o.node {
	case nodeArch:
		return "ForArch(" + merge.QuoteBounded(string(o.arch)) + ")"
	case nodeHost:
		return "ForHost()"
	case nodeAnyArch:
		return "ForAnyArch()"
	case nodeNone:
	}

	return "Options{}"
}

// native returns the architecture the merge and the checks imply, the empty
// Arch for one not known (see mayBeNative), or the error for options that
// name no node or an architecture this package does not know.
func (o Options) native() (specs.Arch, error) {
	switch o.node {
	case nodeAnyArch:
		return "", nil
	case nodeHost:
		return runningArchitecture(), nil
	case nodeArch:
		if !isKnownArch(o.arch) {
			return "", fmt.Errorf(
				"%w: native architecture: %s: %w",
				ErrInvalidOptions, merge.QuoteBounded(string(o.arch)), ErrUnknownArch,
			)
		}

		return o.arch, nil
	case nodeNone:
	}

	return "", fmt.Errorf(
		"%w: seccomp options name no node: use ForArch, ForHost or ForAnyArch", ErrInvalidOptions,
	)
}
