//go:build seccomp && linux && cgo

package seccomp

import (
	"slices"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
)

// safeShape restates the clause sets of a base profile that libseccomp
// decides by an order of its own as the shapes the merge library reads
// exactly (see the evaluation model of its seccomp package); an intersection
// reads any other set as its most restrictive action and so denies the
// syscall. Unconditional clauses are exact: libseccomp keeps the first and
// drops the conditional ones. The two sets of the default profile that remain
// become a single conditional clause each, the one shape that also survives
// on architectures that route socket through socketcall, with a known
// difference: clone with a namespace flag and socket(AF_VSOCK) fail with the
// default errno instead of EPERM, and socket(AF_NETLINK, _, NETLINK_AUDIT)
// is allowed instead of failing with EINVAL, which libaudit took as "no
// audit" rather than failing later with EPERM.
// Other sets are left alone, so that mergeableBase can report them.
func safeShape(profile *rspec.LinuxSeccomp) *rspec.LinuxSeccomp {
	if profile == nil {
		return nil
	}

	result := *profile
	result.Syscalls = make([]rspec.LinuxSyscall, len(profile.Syscalls))

	var names []string

	for i, sc := range profile.Syscalls {
		result.Syscalls[i] = sc
		result.Syscalls[i].Names = slices.Clone(sc.Names)
		result.Syscalls[i].Args = slices.Clone(sc.Args)

		for _, name := range sc.Names {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}

	for _, name := range names {
		var entries []int

		for i := range result.Syscalls {
			if slices.Contains(result.Syscalls[i].Names, name) {
				entries = append(entries, i)
			}
		}

		if len(entries) < 2 {
			continue
		}

		keep := -1

		if first := slices.IndexFunc(entries, func(i int) bool {
			return len(result.Syscalls[i].Args) == 0
		}); first >= 0 {
			keep = entries[first]
		} else {
			switch name {
			case "clone":
				keep = namespaceFlagAllowClause(result.Syscalls, entries)
			case "socket":
				if deny, allow := addressFamilyClauses(result.Syscalls, entries); deny >= 0 {
					if allow < 0 {
						result.Syscalls = append(result.Syscalls, rspec.LinuxSyscall{
							Names:  []string{name},
							Action: rspec.ActAllow,
							Args: []rspec.LinuxSeccompArg{{
								Index: 0,
								Value: result.Syscalls[deny].Args[0].Value,
								Op:    rspec.OpNotEqual,
							}},
						})
						allow = len(result.Syscalls) - 1
					}

					keep = allow
				}
			}
		}

		if keep < 0 {
			continue
		}

		for _, i := range entries {
			if i != keep {
				result.Syscalls[i].Names = slices.DeleteFunc(result.Syscalls[i].Names, func(n string) bool {
					return n == name
				})
			}
		}
	}

	result.Syscalls = slices.DeleteFunc(result.Syscalls, func(sc rspec.LinuxSyscall) bool {
		return len(sc.Names) == 0
	})

	return &result
}

// namespaceFlagAllowClause finds the clone rules of the default profile: one
// clause that allows calls without namespace flags (a masked comparison) next
// to clauses that fail each flag with EPERM. Only the allowing clause is a
// safe shape; calls with a flag then get the default errno.
func namespaceFlagAllowClause(syscalls []rspec.LinuxSyscall, entries []int) int {
	allow := -1

	for _, i := range entries {
		sc := syscalls[i]
		masked := len(sc.Args) > 0 && !slices.ContainsFunc(sc.Args, func(arg rspec.LinuxSeccompArg) bool {
			return arg.Op != rspec.OpMaskedEqual || arg.Index != sc.Args[0].Index
		})

		switch {
		case !masked:
			return -1
		case sc.Action == rspec.ActAllow && len(sc.Args) == 1 && allow < 0:
			allow = i
		case sc.Action == rspec.ActErrno:
		default:
			return -1
		}
	}

	return allow
}

// addressFamilyClauses finds the socket rules of the default profile: one
// address family denied, and either everything else allowed in one clause
// (with CAP_AUDIT_WRITE) or another family denied for one protocol and
// allowed otherwise, everything else allowed (without). It returns the
// denying clause and the clause allowing every other family, or -1 for one
// that is missing. Allowing every other family in one clause leaves the
// denied one to the default action.
func addressFamilyClauses(syscalls []rspec.LinuxSyscall, entries []int) (deny, allow int) {
	deny, allow = -1, -1

	var (
		protocolDeny, protocolAllow bool
	)

	for _, i := range entries {
		sc := syscalls[i]
		eq := func(k int) bool { return k < len(sc.Args) && sc.Args[k].Op == rspec.OpEqualTo }
		ne := func(k int) bool { return k < len(sc.Args) && sc.Args[k].Op == rspec.OpNotEqual }

		switch {
		case sc.Action == rspec.ActErrno && len(sc.Args) == 1 && eq(0) && sc.Args[0].Index == 0 && deny < 0:
			deny = i
		case sc.Action == rspec.ActErrno && len(sc.Args) == 2 && eq(0) && eq(1) && !protocolDeny:
			protocolDeny = true
		case sc.Action == rspec.ActAllow && len(sc.Args) == 2 && eq(0) && ne(1) && !protocolAllow:
			protocolAllow = true
		case sc.Action == rspec.ActAllow && len(sc.Args) == 1 && ne(0) && sc.Args[0].Index == 0 && allow < 0:
			allow = i
		default:
			return -1, -1
		}
	}

	switch {
	case deny < 0:
		return -1, -1
	case allow >= 0 && len(entries) == 2 && syscalls[allow].Args[0].Value == syscalls[deny].Args[0].Value:
		// ERRNO a0 == X next to ALLOW a0 != X: the complement pair.
		return deny, allow
	case allow >= 0 && protocolDeny && protocolAllow && len(entries) == 4:
		// The clause allowing every other family tests the protocol family,
		// not the denied one; a new clause replaces it.
		return deny, -1
	default:
		return -1, -1
	}
}
