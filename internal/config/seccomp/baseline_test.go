//go:build seccomp && linux && cgo

package seccomp

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"go.podman.io/common/pkg/seccomp"
	"golang.org/x/sys/unix"
	spmseccomp "sigs.k8s.io/security-profiles-merger/seccomp"
)

// defaultCaps are the capabilities of CRI-O's default configuration.
var defaultCaps = []string{
	"CAP_CHOWN",
	"CAP_DAC_OVERRIDE",
	"CAP_FSETID",
	"CAP_FOWNER",
	"CAP_SETGID",
	"CAP_SETUID",
	"CAP_SETPCAP",
	"CAP_NET_BIND_SERVICE",
	"CAP_KILL",
}

// renderForArch renders a profile for a container with the capabilities as
// LoadProfileFromConfig does on a node of the architecture, which GOARCH and
// libseccomp name alike for the architectures CRI-O supports: with the
// entries for that architecture and the architectures of its archMap entry,
// if any.
func renderForArch(
	t *testing.T, profile *seccomp.Seccomp, goarch string, caps []string,
) *rspec.LinuxSeccomp {
	t.Helper()

	native := map[string]seccomp.Arch{
		"amd64":   seccomp.ArchX86_64,
		"arm64":   seccomp.ArchAARCH64,
		"ppc64le": seccomp.ArchPPC64LE,
		"s390x":   seccomp.ArchS390X,
	}[goarch]

	forArch := *profile
	forArch.ArchMap = nil
	forArch.Architectures = nil
	forArch.Syscalls = nil

	for _, a := range profile.ArchMap {
		if a.Arch == native {
			forArch.Architectures = append([]seccomp.Arch{a.Arch}, a.SubArches...)
		}
	}

	for _, sc := range profile.Syscalls {
		if len(sc.Includes.Arches) > 0 && !slices.Contains(sc.Includes.Arches, goarch) ||
			slices.Contains(sc.Excludes.Arches, goarch) {
			continue
		}

		entry := *sc
		entry.Includes.Arches, entry.Excludes.Arches = nil, nil
		forArch.Syscalls = append(forArch.Syscalls, &entry)
	}

	rendered, err := seccomp.LoadProfileFromConfig(&forArch, &rspec.Spec{
		Process: &rspec.Process{
			Capabilities: &rspec.LinuxCapabilities{Bounding: caps},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	return rendered
}

// TestDefaultProfileIsValidBaseline checks that the default profile, as the
// configuration loads it, is a valid baseline on this node.
func TestDefaultProfileIsValidBaseline(t *testing.T) {
	if err := validateBaseline(DefaultProfile(), false); err != nil {
		t.Fatal(err)
	}
}

// TestSafeShapeDefaultProfile checks that the default profile of each
// architecture CRI-O supports, restated by safeShape, passes ValidateBaseline
// for that architecture, so that no merge takes anything away from it: the
// shapes of its clone and socket rules would otherwise deny both. The flags
// of clone are the second argument on s390x. CAP_SYS_ADMIN allows clone and
// clone3 unconditionally, and CAP_AUDIT_WRITE leaves socket with a
// complement pair instead of four entries.
func TestSafeShapeDefaultProfile(t *testing.T) {
	withCap := func(c string) []string {
		return append(slices.Clone(defaultCaps), c)
	}

	for goarch, arch := range map[string]rspec.Arch{
		"amd64":   rspec.ArchX86_64,
		"arm64":   rspec.ArchAARCH64,
		"ppc64le": rspec.ArchPPC64LE,
		"s390x":   rspec.ArchS390X,
	} {
		for name, caps := range map[string][]string{
			"default caps":    defaultCaps,
			"CAP_SYS_ADMIN":   withCap("CAP_SYS_ADMIN"),
			"CAP_AUDIT_WRITE": withCap("CAP_AUDIT_WRITE"),
		} {
			t.Run(goarch+" with "+name, func(t *testing.T) {
				sysAdmin := slices.Contains(caps, "CAP_SYS_ADMIN")
				options := spmseccomp.ForArch(arch)
				rendered := renderForArch(t, defaultProfileForArch(goarch), goarch, caps)

				err := options.ValidateBaseline(rendered)
				if !errors.Is(err, spmseccomp.ErrInexactBaseline) {
					t.Fatalf(
						"expected the rendered default profile to be an inexact baseline, got %v",
						err,
					)
				}

				base := safeShape(rendered)
				if err := options.ValidateBaseline(base); err != nil {
					t.Fatalf("unexpected error for the restated default profile: %v", err)
				}

				if err := options.ValidateArtifact(base); err != nil {
					t.Fatalf(
						"unexpected conflicting entries in the restated default profile: %v",
						err,
					)
				}

				merged, err := options.Intersect(base, &rspec.LinuxSeccomp{
					DefaultAction: rspec.ActAllow,
					Architectures: base.Architectures,
				})
				if err != nil {
					t.Fatal(err)
				}

				rules := func(name string) []rspec.LinuxSyscall {
					var found []rspec.LinuxSyscall

					for _, sc := range merged.Syscalls {
						if slices.Contains(sc.Names, name) {
							found = append(found, sc)
						}
					}

					return found
				}

				flagsIndex := uint(0)
				if goarch == "s390x" {
					flagsIndex = 1
				}

				flags := []rspec.LinuxSeccompArg{
					{
						Index: flagsIndex,
						Value: unix.CLONE_NEWNS | unix.CLONE_NEWUTS | unix.CLONE_NEWIPC | unix.CLONE_NEWUSER |
							unix.CLONE_NEWPID | unix.CLONE_NEWNET | unix.CLONE_NEWCGROUP,
						Op: rspec.OpMaskedEqual,
					},
				}
				vsock := []rspec.LinuxSeccompArg{
					{Index: 0, Value: unix.AF_VSOCK, Op: rspec.OpNotEqual},
				}

				allow := func(args []rspec.LinuxSeccompArg) []rspec.LinuxSyscall {
					return []rspec.LinuxSyscall{{Action: rspec.ActAllow, Args: args}}
				}

				expected := map[string][]rspec.LinuxSyscall{
					"fork":   allow(nil),
					"setns":  allow(nil),
					"socket": allow(vsock),
					"clone":  allow(flags),
					"clone3": nil,
				}
				if sysAdmin {
					expected["clone"] = allow(nil)
					expected["clone3"] = allow(nil)
				}

				for name, want := range expected {
					var got []rspec.LinuxSyscall
					for _, sc := range rules(name) {
						got = append(got, rspec.LinuxSyscall{Action: sc.Action, Args: sc.Args})
					}

					if !reflect.DeepEqual(got, want) {
						t.Errorf("%s: expected %v, got %v", name, want, got)
					}
				}
			})
		}
	}
}

// TestSafeShapeKeepsDenials checks that restating a base profile never allows
// what the profile denies: the restatement of the clone and socket rules
// leaves the denied calls to the default action, which only denies them in a
// profile that fails calls by default.
func TestSafeShapeKeepsDenials(t *testing.T) {
	enosys := uint(unix.ENOSYS)
	namespaces := uint64(unix.CLONE_NEWUSER | unix.CLONE_NEWNS)

	arg := func(index uint, value, valueTwo uint64, op rspec.LinuxSeccompOperator) rspec.LinuxSeccompArg {
		return rspec.LinuxSeccompArg{Index: index, Value: value, ValueTwo: valueTwo, Op: op}
	}
	clause := func(name string, action rspec.LinuxSeccompAction, args ...rspec.LinuxSeccompArg) rspec.LinuxSyscall {
		return rspec.LinuxSyscall{Names: []string{name}, Action: action, Args: args}
	}

	vsockDenied := clause("socket", rspec.ActErrno, arg(0, unix.AF_VSOCK, 0, rspec.OpEqualTo))
	vsockOthers := clause("socket", rspec.ActAllow, arg(0, unix.AF_VSOCK, 0, rspec.OpNotEqual))
	cloneDenied := clause("clone", rspec.ActErrno,
		arg(0, unix.CLONE_NEWUSER, unix.CLONE_NEWUSER, rspec.OpMaskedEqual),
		arg(0, unix.CLONE_NEWNS, unix.CLONE_NEWNS, rspec.OpMaskedEqual),
	)

	for _, tc := range []struct {
		name string
		base *rspec.LinuxSeccomp
		// syscall is the one the base profile restricts, and expected the
		// clauses a merge with a profile allowing everything leaves of it.
		// A base profile that is rejected instead is fine as well.
		syscall  string
		expected []rspec.LinuxSyscall
	}{
		{
			name: "default allow with a denied address family",
			base: &rspec.LinuxSeccomp{
				DefaultAction: rspec.ActAllow,
				Syscalls:      []rspec.LinuxSyscall{vsockDenied, vsockOthers},
			},
			syscall:  "socket",
			expected: []rspec.LinuxSyscall{vsockDenied},
		},
		{
			name: "default allow with an unconditional clause repeating it",
			base: &rspec.LinuxSeccomp{
				DefaultAction: rspec.ActAllow,
				Syscalls:      []rspec.LinuxSyscall{clause("socket", rspec.ActAllow), vsockDenied},
			},
			syscall:  "socket",
			expected: []rspec.LinuxSyscall{vsockDenied},
		},
		{
			name: "default allow with denied namespace flags",
			base: &rspec.LinuxSeccomp{
				DefaultAction: rspec.ActAllow,
				Syscalls: []rspec.LinuxSyscall{
					clause("clone", rspec.ActAllow, arg(0, namespaces, 0, rspec.OpMaskedEqual)),
					cloneDenied,
				},
			},
			syscall: "clone",
			expected: []rspec.LinuxSyscall{
				clause("clone", rspec.ActErrno, cloneDenied.Args[0]),
				clause("clone", rspec.ActErrno, cloneDenied.Args[1]),
			},
		},
		{
			name: "default kill with a denied address family",
			base: &rspec.LinuxSeccomp{
				DefaultAction: rspec.ActKillProcess,
				Syscalls:      []rspec.LinuxSyscall{vsockDenied, vsockOthers},
			},
			syscall:  "socket",
			expected: []rspec.LinuxSyscall{vsockDenied, vsockOthers},
		},
		{
			name: "allowed namespace flags that do not cover the denied ones",
			base: &rspec.LinuxSeccomp{
				DefaultAction:   rspec.ActErrno,
				DefaultErrnoRet: &enosys,
				Syscalls: []rspec.LinuxSyscall{
					clause("clone", rspec.ActAllow, arg(0, unix.CLONE_NEWPID, 0, rspec.OpMaskedEqual)),
					cloneDenied,
				},
			},
			syscall: "clone",
		},
		{
			name: "address families of another profile than the default",
			base: &rspec.LinuxSeccomp{
				DefaultAction:   rspec.ActErrno,
				DefaultErrnoRet: &enosys,
				Syscalls: []rspec.LinuxSyscall{
					vsockDenied,
					clause("socket", rspec.ActErrno,
						arg(0, unix.AF_NETLINK, 0, rspec.OpEqualTo), arg(2, unix.NETLINK_AUDIT, 0, rspec.OpEqualTo)),
					clause("socket", rspec.ActAllow,
						arg(0, unix.AF_NETLINK, 0, rspec.OpEqualTo), arg(2, unix.NETLINK_AUDIT, 0, rspec.OpNotEqual)),
					// AF_PACKET is left to the default action.
					clause("socket", rspec.ActAllow, arg(0, unix.AF_PACKET, 0, rspec.OpNotEqual)),
				},
			},
			syscall: "socket",
		},
		{
			name: "default errno with an unconditional clause repeating it",
			base: &rspec.LinuxSeccomp{
				DefaultAction:   rspec.ActErrno,
				DefaultErrnoRet: &enosys,
				Syscalls: []rspec.LinuxSyscall{
					{Names: []string{"socket"}, Action: rspec.ActErrno, ErrnoRet: &enosys},
					clause("socket", rspec.ActAllow, arg(0, unix.AF_UNIX, 0, rspec.OpEqualTo)),
				},
			},
			syscall: "socket",
			expected: []rspec.LinuxSyscall{
				clause("socket", rspec.ActAllow, arg(0, unix.AF_UNIX, 0, rspec.OpEqualTo)),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, err := mergeableBase(tc.base)
			if err != nil {
				t.Logf("rejected: %v", err)

				return
			}

			merged, err := spmseccomp.Intersect(
				base,
				&rspec.LinuxSeccomp{DefaultAction: rspec.ActAllow},
			)
			if err != nil {
				t.Fatal(err)
			}

			if tc.expected == nil {
				t.Fatalf(
					"expected the base profile to be rejected, got %s",
					spmseccomp.FormatProfile(merged),
				)
			}

			var got []rspec.LinuxSyscall

			for _, sc := range merged.Syscalls {
				if slices.Contains(sc.Names, tc.syscall) {
					got = append(got, clause(tc.syscall, sc.Action, sc.Args...))
				}
			}

			// A default action that no clause has compares the clauses
			// as they are.
			diff, err := spmseccomp.Diff(
				&rspec.LinuxSeccomp{DefaultAction: rspec.ActKillProcess, Syscalls: tc.expected},
				&rspec.LinuxSeccomp{DefaultAction: rspec.ActKillProcess, Syscalls: got},
			)
			if err != nil {
				t.Fatal(err)
			}

			if !diff.Equal {
				t.Errorf("expected %s of the merge to stay as the base profile has it, got %s",
					tc.syscall, spmseccomp.FormatProfile(merged))
			}
		})
	}
}
