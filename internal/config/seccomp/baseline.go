//go:build seccomp && linux && cgo

package seccomp

import (
	"slices"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
)

// safeShape restates the clause sets of a base profile that libseccomp
// decides by an order of its own as the shapes the merge library reads
// exactly (see the evaluation model of its seccomp package); an intersection
// reads any other set as its most restrictive action and so denies the
// syscall. Clauses that repeat the default action are not part of a set,
// because runc and crun do not load them. Unconditional clauses are exact:
// libseccomp keeps the first and drops the conditional ones. The two sets of
// the default profile that remain become a single conditional clause each,
// the one shape that also survives on architectures that route socket through
// socketcall, with a known difference: clone with a namespace flag and
// socket(AF_VSOCK) fail with the default errno instead of EPERM, and
// socket(AF_NETLINK, _, NETLINK_AUDIT) is allowed instead of failing with
// EINVAL, which libaudit took as "no audit" rather than failing later with
// EPERM. That leaves the denied calls to the default action, so it only
// applies to a profile that fails calls by default, as the default profile
// does: under any other default action the restated profile would allow
// them, or kill where the profile asked for an error.
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
			if slices.Contains(result.Syscalls[i].Names, name) &&
				!repeatsDefault(profile, &result.Syscalls[i]) {
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
		} else if profile.DefaultAction == rspec.ActErrno {
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
				result.Syscalls[i].Names = slices.DeleteFunc(
					result.Syscalls[i].Names,
					func(n string) bool {
						return n == name
					},
				)
			}
		}
	}

	result.Syscalls = slices.DeleteFunc(result.Syscalls, func(sc rspec.LinuxSyscall) bool {
		return len(sc.Names) == 0
	})

	return &result
}

// repeatsDefault returns true if a clause has the result of the default
// action, errno included. runc and crun skip such a clause instead of loading
// it, so it decides nothing and hides no other clause of its syscalls.
func repeatsDefault(profile *rspec.LinuxSeccomp, sc *rspec.LinuxSyscall) bool {
	if loadedAction(sc.Action) != loadedAction(profile.DefaultAction) {
		return false
	}

	if sc.Action != rspec.ActErrno && sc.Action != rspec.ActTrace {
		return true
	}

	return loadedErrno(sc.ErrnoRet) == loadedErrno(profile.DefaultErrnoRet)
}

// loadedAction returns the action as libseccomp knows it, to which
// SCMP_ACT_KILL and SCMP_ACT_KILL_THREAD are the same.
func loadedAction(action rspec.LinuxSeccompAction) rspec.LinuxSeccompAction {
	if action == rspec.ActKillThread {
		return rspec.ActKill
	}

	return action
}

// loadedErrno returns the errno runc and crun load: EPERM if unset, and
// otherwise the 16 bits that fit the action.
func loadedErrno(errnoRet *uint) uint {
	if errnoRet == nil {
		return uint(unix.EPERM)
	}

	return *errnoRet & 0xffff
}

// namespaceFlagAllowClause finds the clone rules of the default profile: one
// clause that allows calls without namespace flags (a masked comparison with
// zero) next to clauses that fail each flag with EPERM. Only the allowing
// clause is a safe shape; calls with a flag then get the default errno. A
// failing clause has to test for a flag of the allowing clause, so that no
// call it fails is one the allowing clause matches.
func namespaceFlagAllowClause(syscalls []rspec.LinuxSyscall, entries []int) int {
	allow := -1

	for _, i := range entries {
		sc := syscalls[i]
		if sc.Action == rspec.ActAllow && len(sc.Args) == 1 &&
			sc.Args[0].Op == rspec.OpMaskedEqual && sc.Args[0].Value&sc.Args[0].ValueTwo == 0 {
			allow = i

			break
		}
	}

	if allow < 0 {
		return -1
	}

	flags := syscalls[allow].Args[0]

	for _, i := range entries {
		if i == allow {
			continue
		}

		sc := syscalls[i]
		if sc.Action != rspec.ActErrno || len(sc.Args) == 0 ||
			slices.ContainsFunc(sc.Args, func(arg rspec.LinuxSeccompArg) bool {
				return arg.Op != rspec.OpMaskedEqual || arg.Index != flags.Index ||
					arg.Value&arg.ValueTwo&flags.Value == 0
			}) {
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
	protocolDeny, protocolAllow := -1, -1

	for _, i := range entries {
		sc := syscalls[i]
		eq := func(k int) bool { return k < len(sc.Args) && sc.Args[k].Op == rspec.OpEqualTo }
		ne := func(k int) bool { return k < len(sc.Args) && sc.Args[k].Op == rspec.OpNotEqual }

		// Every clause tests the address family first.
		if len(sc.Args) == 0 || sc.Args[0].Index != 0 {
			return -1, -1
		}

		switch {
		case sc.Action == rspec.ActErrno && len(sc.Args) == 1 && eq(0) && deny < 0:
			deny = i
		case sc.Action == rspec.ActErrno && len(sc.Args) == 2 && eq(0) && eq(1) && protocolDeny < 0:
			protocolDeny = i
		case sc.Action == rspec.ActAllow && len(sc.Args) == 2 && eq(0) && ne(1) && protocolAllow < 0:
			protocolAllow = i
		case sc.Action == rspec.ActAllow && len(sc.Args) == 1 && ne(0) && allow < 0:
			allow = i
		default:
			return -1, -1
		}
	}

	if deny < 0 || allow < 0 {
		return -1, -1
	}

	denied, other := syscalls[deny].Args[0].Value, syscalls[allow].Args[0].Value

	switch {
	case len(entries) == 2 && other == denied:
		// ERRNO a0 == X next to ALLOW a0 != X: the complement pair.
		return deny, allow
	case len(entries) == 4 && protocolDeny >= 0 && protocolAllow >= 0 && other != denied &&
		syscalls[protocolDeny].Args[0].Value == other &&
		syscalls[protocolAllow].Args[0].Value == other &&
		syscalls[protocolDeny].Args[1].Index == syscalls[protocolAllow].Args[1].Index &&
		syscalls[protocolDeny].Args[1].Value == syscalls[protocolAllow].Args[1].Value:
		// The clause allowing every other family tests the family with the
		// denied protocol, not the denied family; a new clause replaces it.
		return deny, -1
	default:
		return -1, -1
	}
}
