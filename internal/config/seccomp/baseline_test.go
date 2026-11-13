//go:build seccomp && linux && cgo

package seccomp

import (
	"slices"
	"testing"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"go.podman.io/common/pkg/seccomp"
	"golang.org/x/sys/unix"
	spmseccomp "sigs.k8s.io/security-profiles-merger/seccomp"
)

// TestSafeShapeDefaultProfile checks that the default profile, restated by
// safeShape, survives an intersection with a profile that allows everything:
// the shapes of its clone and socket rules would otherwise deny both.
func TestSafeShapeDefaultProfile(t *testing.T) {
	caps := []string{
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
	spec := &rspec.Spec{
		Process: &rspec.Process{
			Capabilities: &rspec.LinuxCapabilities{
				Bounding:  caps,
				Effective: caps,
				Permitted: caps,
			},
		},
	}

	rendered, err := seccomp.LoadProfileFromConfig(DefaultProfile(), spec)
	if err != nil {
		t.Fatal(err)
	}

	if err := spmseccomp.ValidateArtifact(rendered); err == nil {
		t.Fatal("expected the rendered default profile to have conflicting entries")
	}

	base, err := mergeableBase(rendered)
	if err != nil {
		t.Fatalf("unexpected error for the restated default profile: %v", err)
	}

	if err := spmseccomp.ValidateArtifact(base); err != nil {
		t.Fatalf("unexpected conflicting entries in the restated default profile: %v", err)
	}

	merged, err := spmseccomp.Intersect(base, &rspec.LinuxSeccomp{DefaultAction: rspec.ActAllow})
	if err != nil {
		t.Fatal(err)
	}

	actions := func(name string) []rspec.LinuxSeccompAction {
		var found []rspec.LinuxSeccompAction

		for _, sc := range merged.Syscalls {
			if slices.Contains(sc.Names, name) {
				found = append(found, sc.Action)
			}
		}

		return found
	}

	for name, expected := range map[string][]rspec.LinuxSeccompAction{
		"fork":   {rspec.ActAllow},
		"clone":  {rspec.ActAllow},
		"setns":  {rspec.ActAllow},
		"socket": {rspec.ActAllow},
		"clone3": nil,
	} {
		got := actions(name)
		slices.Sort(got)
		slices.Sort(expected)

		if !slices.Equal(got, expected) {
			t.Errorf("%s: expected actions %v, got %v", name, expected, got)
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

			if spmseccomp.DiffSyscalls(tc.expected, got) != nil {
				t.Errorf("expected %s of the merge to stay as the base profile has it, got %s",
					tc.syscall, spmseccomp.FormatProfile(merged))
			}
		})
	}
}
