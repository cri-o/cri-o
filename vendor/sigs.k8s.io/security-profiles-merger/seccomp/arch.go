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
	"runtime"
	"slices"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// NativeArchitecture returns the seccomp architecture of the running
// program, derived from runtime.GOARCH, and false when GOARCH has none. The
// merges account for it without help; it is for callers that want to spell
// it out.
func NativeArchitecture() (specs.Arch, bool) {
	arch, ok := nativeArchitectures[runtime.GOARCH]

	return arch, ok
}

// nativeArchitectures maps runtime.GOARCH values to seccomp architectures.
var nativeArchitectures = map[string]specs.Arch{
	"386":      specs.ArchX86,
	"amd64":    specs.ArchX86_64,
	"arm":      specs.ArchARM,
	"arm64":    specs.ArchAARCH64,
	"loong64":  specs.ArchLOONGARCH64,
	"mips":     specs.ArchMIPS,
	"mips64":   specs.ArchMIPS64,
	"mips64le": specs.ArchMIPSEL64,
	"mipsle":   specs.ArchMIPSEL,
	"ppc64":    specs.ArchPPC64,
	"ppc64le":  specs.ArchPPC64LE,
	"riscv64":  specs.ArchRISCV64,
	"s390x":    specs.ArchS390X,
}

// narrowArchitectures are the architectures for which libseccomp compiles
// 32-bit argument comparisons: it drops the upper 32 bits of every value and
// mask before comparing, so a condition against a value above 32 bits tests
// something else there (SCMP_CMP_EQ against 0x100000005 matches 5, and a
// SCMP_CMP_MASKED_EQ whose mask sets only upper bits matches every call).
// x32 and the MIPS n32 ABIs count as 32-bit here, as libseccomp compiles
// them that way. Checked against libseccomp 2.6.1 with every architecture
// it knows.
var narrowArchitectures = map[specs.Arch]bool{
	specs.ArchX86:         true,
	specs.ArchX32:         true,
	specs.ArchARM:         true,
	specs.ArchMIPS:        true,
	specs.ArchMIPSEL:      true,
	specs.ArchMIPS64N32:   true,
	specs.ArchMIPSEL64N32: true,
	specs.ArchPPC:         true,
	specs.ArchS390:        true,
	specs.ArchPARISC:      true,
	specs.ArchM68K:        true,
	specs.ArchSH:          true,
	specs.ArchSHEB:        true,
}

// multiplexingArchitectures are the architectures on which libseccomp adds
// a rule for a socket or SysV IPC syscall a second time, on the multiplexer
// socketcall(2) or ipc(2) (see multiplexedSyscalls). Checked against
// libseccomp 2.6.1 with every architecture it knows.
var multiplexingArchitectures = map[specs.Arch]bool{
	specs.ArchX86:     true,
	specs.ArchMIPS:    true,
	specs.ArchMIPSEL:  true,
	specs.ArchPPC:     true,
	specs.ArchPPC64:   true,
	specs.ArchPPC64LE: true,
	specs.ArchS390:    true,
	specs.ArchS390X:   true,
	specs.ArchM68K:    true,
	specs.ArchSH:      true,
	specs.ArchSHEB:    true,
}

// byteOrder is the byte order of an architecture. The zero value is none,
// for an architecture this package does not know.
type byteOrder int

const (
	littleEndian byteOrder = iota + 1
	bigEndian
)

// String names the byte order.
func (order byteOrder) String() string {
	switch order {
	case littleEndian:
		return "little-endian"
	case bigEndian:
		return "big-endian"
	default:
		return "unknown byte order"
	}
}

// byteOrders gives the byte order of every architecture this package knows,
// each stated, so that one missing from it is caught rather than taken for
// either. libseccomp keeps one byte order per filter and refuses an
// architecture of the other one with EDOM, so a runtime fails to load a
// profile listing an architecture whose byte order differs from the native
// architecture's.
var byteOrders = map[specs.Arch]byteOrder{
	specs.ArchX86:         littleEndian,
	specs.ArchX86_64:      littleEndian,
	specs.ArchX32:         littleEndian,
	specs.ArchARM:         littleEndian,
	specs.ArchAARCH64:     littleEndian,
	specs.ArchMIPS:        bigEndian,
	specs.ArchMIPS64:      bigEndian,
	specs.ArchMIPS64N32:   bigEndian,
	specs.ArchMIPSEL:      littleEndian,
	specs.ArchMIPSEL64:    littleEndian,
	specs.ArchMIPSEL64N32: littleEndian,
	specs.ArchPPC:         bigEndian,
	specs.ArchPPC64:       bigEndian,
	specs.ArchPPC64LE:     littleEndian,
	specs.ArchS390:        bigEndian,
	specs.ArchS390X:       bigEndian,
	specs.ArchPARISC:      bigEndian,
	specs.ArchPARISC64:    bigEndian,
	specs.ArchRISCV64:     littleEndian,
	specs.ArchLOONGARCH64: littleEndian,
	specs.ArchM68K:        bigEndian,
	specs.ArchSH:          littleEndian,
	specs.ArchSHEB:        bigEndian,
}

// runningArchitecture returns the native architecture of the running
// program, or the empty Arch where it has none.
func runningArchitecture() specs.Arch {
	native, _ := NativeArchitecture()

	return native
}

// coversAny reports whether a filter loaded from a profile listing archs
// covers an architecture of the given set: one it lists, or native, which
// runtimes always add (see mayBeNative). The merge and ValidateArtifact run
// where the runtime loading their result runs, so they pass the native
// architecture of the running program.
func coversAny(archs []specs.Arch, set map[specs.Arch]bool, native specs.Arch) bool {
	return mayBeNative(set, native) ||
		slices.ContainsFunc(archs, func(arch specs.Arch) bool { return set[arch] })
}

// mayBeNative reports whether the architecture a runtime adds to the filter
// may be one of the set: native itself, or, where native is empty and so
// not known, any architecture this package knows (see possibleNatives).
// Those include 32-bit and multiplexing ones, so a result settled for an
// unknown native architecture is settled for both, which keeps it safe on
// whichever node loads it.
func mayBeNative(set map[specs.Arch]bool, native specs.Arch) bool {
	if native == "" {
		return slices.ContainsFunc(possibleNatives, func(arch specs.Arch) bool { return set[arch] })
	}

	return set[native]
}
