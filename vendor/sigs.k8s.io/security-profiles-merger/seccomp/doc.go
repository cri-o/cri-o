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

// Package seccomp merges, compares and validates seccomp profiles in the
// [specs.LinuxSeccomp] form of the OCI runtime-spec.
//
// [Intersect] produces a profile that permits a syscall only where every
// input permits it, the merge a CRI runtime runs on an artifact (a profile
// pulled from an OCI artifact) and its baseline (KEP-6061). [Union] produces
// one that permits a syscall where any input does, the merge the Security
// Profiles Operator runs on recorded profiles. [Diff] compares two profiles
// in the same terms.
//
// [Validate] checks what a runtime needs to load a profile at all, and both
// merges run it on every input. [ValidateStrict] adds the checks worth
// making on a profile a person wrote, and [ValidateArtifact] the ones a
// runtime applies to an artifact. [UnmarshalStrict] decodes a profile and
// refuses what encoding/json accepts silently, [Marshal] writes one as a
// runtime is to read it, [DecodeArtifact] decodes and checks an artifact in
// one call, and [ValidateBaseline] checks a baseline once, at config load.
//
// What the merges, [Diff] and the artifact checks return depends on the
// native architecture of the node that loads the result. The functions
// assume the node the program runs on; the methods of [Options] take another
// (see the Architectures section). CONTRIBUTING.md records how the claims
// below about libseccomp, runc and crun were checked.
//
// # Action order
//
// Actions are ranked from most to least restrictive:
//
//	SCMP_ACT_KILL_PROCESS > SCMP_ACT_KILL_THREAD > SCMP_ACT_TRAP >
//	SCMP_ACT_ERRNO > SCMP_ACT_NOTIFY > SCMP_ACT_TRACE > SCMP_ACT_LOG >
//	SCMP_ACT_ALLOW
//
// SCMP_ACT_KILL ranks with SCMP_ACT_KILL_THREAD, which libseccomp defines as
// the same action, and results spell both SCMP_ACT_KILL. Default actions are
// merged with the same ranking. [Validate], and so both merges, reject
// unknown actions ([ErrUnknownAction]).
//
// # Evaluation model
//
// "Permit" is judged by what runc and crun load through libseccomp and what
// the kernel then runs, not by the order of the entries. runc and crun add
// one rule per entry and syscall name, skipping entries whose action and
// errno equal the profile default, and runc adds an entry that repeats an
// argument index as one rule per condition.
//
// libseccomp evaluates the rules of a syscall in an order of its own,
// whatever the actions, refuses some rule sets, and before 2.6.2 compiles
// others wrongly. The package therefore relies on the compiled program only
// for rule sets in a safe shape:
//
//   - (a) unconditional: libseccomp drops every conditional rule of a
//     syscall that has an unconditional one, whichever is added first, and
//     keeps the first of several unconditional rules;
//   - (b) a single conditional rule;
//   - (c) several rules with one SCMP_CMP_EQ condition each, all on the
//     same argument index, whose values differ even in their lower 32 bits,
//     with any results;
//   - (d) several rules with one condition each and the same result, where
//     no argument index shared by several rules carries a range comparison
//     (SCMP_CMP_LT, SCMP_CMP_LE, SCMP_CMP_GT, SCMP_CMP_GE) against a value
//     above 32 bits, which libseccomp miscompiles;
//   - (e) two rules with one condition each on the same argument index and
//     value, whose operators are complements (SCMP_CMP_EQ and SCMP_CMP_NE,
//     SCMP_CMP_LT and SCMP_CMP_GE, SCMP_CMP_LE and SCMP_CMP_GT), with any
//     results.
//
// In a safe shape a call gets the result of the rule it matches, or the
// default when it matches none. Any other rule set is read as one
// unconditional rule with the most restrictive (intersection) or least
// restrictive (union) of its actions and the default, since a call gets one
// of those whatever libseccomp does. That includes rules sharing one result
// with several conditions each: an artifact allowing read if a0 == 1 &&
// a1 == 2 and read if a0 == 3 && a1 == 4 under default SCMP_ACT_ERRNO
// passes [ValidateArtifact], which rejects only rule sets with different
// results, and Intersect of it denies every read.
//
// A baseline is read the same way, so one that relies on libseccomp's order
// loses the syscall in an intersection. The default profile of CRI-O and
// Podman is one: its socket rules are in no safe shape, so an intersection
// with it denies every socket call. [ValidateArtifact] names such a syscall
// in a baseline as it does in an artifact ([ErrConflictingEntries]).
//
// Merge results only contain safe shapes, and never an unconditional entry
// next to conditional entries for the same syscall. Where the merged rules
// of a syscall would need both, a single filtered entry with exactly one
// condition whose operator has a complement (any but SCMP_CMP_MASKED_EQ) is
// kept and the unconditional rule becomes an entry for the complementary
// condition (arg0 == 1 and arg0 != 1), which is exact; the filtered entry is
// left out where it equals the default, which then decides its calls. An
// unconditional rule that differs from the default only by errno is dropped
// when filtered entries with a different action remain, so the calls it
// decided, including those of filtered entries with its action and errno,
// get the default's errno instead. Anything else collapses to one
// unconditional entry with the most restrictive (intersection) or least
// restrictive (union) action involved. No entry of a result equals the
// default, so a result can name fewer syscalls than an input. Unless the
// default action is SCMP_ACT_ALLOW, SCMP_ACT_LOG or SCMP_ACT_TRACE, runc
// fails a call above the highest syscall a profile names with ENOSYS, so
// such a call can get ENOSYS where an input applied its default, which
// denies it either way.
//
// Merge results pass [Validate]. They are not meant to pass
// [ValidateStrict], since a result often lists one syscall in several
// entries ([ErrDuplicateSyscallName]), nor [ValidateArtifact], whose
// artifact rules a result need not meet: a baseline's listener carries into
// it, a union can exceed the limits its inputs met, and a union keeps an
// architecture of another byte order than the node's ([ErrMixedByteOrder]).
//
// Argument conditions are compared as libseccomp evaluates them: valueTwo is
// only read for SCMP_CMP_MASKED_EQ, where libseccomp masks it with value,
// and is cleared for every other operator. A SCMP_CMP_MASKED_EQ with an
// empty mask holds for every value and is dropped, as libseccomp drops it.
//
// # Intersection
//
// For each call, [Intersect] picks the more restrictive action of the
// inputs, except on an architecture it drops (see below). Filters on
// different argument indices are conjoined into one entry, identical filters
// are kept, several entries for one syscall (an OR of filters) are
// preserved, and filters that provably never overlap (arg0 == 1 and arg0 ==
// 2) produce no shared entry. Of two conditions on one argument where one
// implies the other, the one that says more is kept (arg0 == 2 against arg0
// != 40 is arg0 == 2, arg0 < 5 against arg0 <= 10 is arg0 < 5), judged only
// on values that fit 32 bits. Where the exact intersection is not
// expressible in OCI terms, such as two conditions on one argument of which
// neither implies the other, the affected calls fall back to the more
// restrictive surrounding action. The result never permits a call that any
// input denies, as libseccomp 2.5.4 to 2.6.2 evaluates the inputs and the
// result, with the limits the Architectures section names.
//
// Where Intersect drops an architecture from the list, libseccomp answers
// every call of it with its action for an unlisted architecture,
// SCMP_ACT_KILL. That denies the call, but where the inputs answer it with
// SCMP_ACT_KILL_PROCESS, the result kills only the thread, since a profile
// has no member for the action on an unlisted architecture. An architecture
// of another byte order than the native one is never dropped; Intersect
// refuses the input listing it.
//
// The result is conservative rather than exact where filters interact: a
// conditional entry is lowered by every overlapping entry of the other side,
// even one covering only part of its region, and a syscall whose rules are
// not in a safe shape collapses as a whole (see the Union section for an
// example).
//
// # Union
//
// [Union] keeps every conditional entry of every input, with its action
// raised to the least restrictive action any input applies to calls matching
// its filter. Where the exact union is not expressible, or would not form a
// safe shape, the result over-approximates in the permissive direction. It
// never denies a call that any input permits, with the limits of Intersect's
// guarantee.
//
// Both directions are conservative where filters interact. Intersecting
// read: ALLOW if arg0 == 1 under default ERRNO with read: ERRNO if arg0 < 5
// and read: TRAP if arg1 == 1 under default ALLOW traps every read, since
// the second profile's rules are not in a safe shape. The union of the same
// inputs allows every read, although both deny arg0 in {0, 2, 3, 4}. The
// guarantees always hold; only the tightness varies.
//
// Two inputs merge to the same actions in either order, errno values aside
// (see the Errno values section). Three or more are merged pairwise, first
// to last, and a conservative step can lose more precision in one order
// than in another, so their order can change how tight the result is, but
// never the guarantee.
//
// # Errno values
//
// An unset errnoRet on SCMP_ACT_ERRNO or SCMP_ACT_TRACE means EPERM, as runc
// and crun apply it, so SCMP_ACT_ERRNO with and without errnoRet 1 are the
// same rule, and errnoRet on any other action is ignored. Results spell
// EPERM as an unset errnoRet and drop values from actions that ignore them.
//
// DefaultErrnoRet is taken from the input whose default action is picked,
// and per-syscall ErrnoRet from the input whose action is picked. When the
// actions tie, the earlier (leftmost) input wins. Whether a conditional
// entry that shares the default action but not its errno survives can
// therefore depend on argument order, so even two inputs give results of
// the same effect in either order only for profiles without errno values.
//
// Rules of one syscall that apply one action, each test one argument for
// equality with a different filter, and report different errno values are
// given the errno of the first of them, so the action still applies to every
// call one of them matches. Two rules that apply one action to the same
// calls, spelled differently, such as a0 < 1 and a0 == 0, are one rule, and
// the merge keeps the errno the leftmost input gives it. A rule set that is
// already a safe shape keeps its errno values as written, and
// [ValidateArtifact] reports such rules in one profile as conflicting rather
// than unifying them.
//
// [Validate] does not range-check errnoRet. [ValidateStrict] and
// [ValidateArtifact] reject the values libseccomp refuses: 4095 (MAX_ERRNO)
// and above on SCMP_ACT_ERRNO, and above 65535 on SCMP_ACT_TRACE, whose
// value is a 16-bit message for the tracer. runc and crun load only the
// lower 16 bits of errnoRet, and the merges and [Diff] read it the same way:
// 65537 is EPERM, so an entry carrying it against a default errno of 1
// equals the default. Results and Diff spell an errno as it is loaded,
// except that Diff reports an errno wider than 16 bits as written, the
// default's and a rule's alike, since its lower bits name a value the
// profile never holds. Diff compares rules as loaded, so one rule can stand
// for several entries writing values that load alike; it shows a rule's
// value as written only where every entry of the syscall with that action
// and loaded value, in both profiles, writes the same one, and as loaded
// otherwise.
//
// # Architectures
//
// The filter always covers the native architecture, and the listed
// architectures are added to it, as runc and crun load them: an empty list
// means "native only", a list need not name the native architecture, and one
// listing only foreign architectures is valid. [Validate] accepts every
// architecture the runtime-spec names, although runc refuses parisc, m68k
// and sh, which crun loads.
//
// libseccomp keeps one byte order per filter, so a runtime fails to load a
// profile listing s390x, ppc64, big-endian MIPS, parisc, m68k or sheb on a
// little-endian node, or a little-endian architecture on a big-endian one.
// [ValidateArtifact] reports such an architecture ([ErrMixedByteOrder]), and
// [Intersect] refuses an input listing one, as an [InputError] wrapping
// ErrMixedByteOrder, rather than apply rules written for another
// architecture. Where the native architecture is not known, both refuse only
// a list mixing the two byte orders, which loads on no node. [Union] keeps
// every architecture an input covers, so its result fails to load as that
// input does.
//
// Intersect takes the set intersection of the lists, which may be empty,
// less the architectures it drops as described below; Union combines them
// exactly. On an architecture only one input lists, the other input applies
// libseccomp's action for an unlisted architecture, SCMP_ACT_KILL, while the
// union applies the merged rules, which is the permissive direction except
// where every input's rules answer a call with SCMP_ACT_KILL_PROCESS: the
// union kills the process there, where the input not listing the
// architecture kills only the thread, and both deny the call.
//
// The evaluation model covers the program libseccomp compiles for a 64-bit
// architecture on which every syscall is called directly. The four effects
// below are read for each architecture a profile lists and for the native
// architecture, which the merges and [Diff] take to be the one of the
// running program (see [NativeArchitecture]) unless [Options] names another.
// The merges are sound for a node linking libseccomp 2.5.4 to 2.6.2; 2.5.2
// evaluates some rule sets differently and is not supported. The lost-rules
// check below knows the syscalls of those releases, and Validate accepts
// names it does not know, so a rule on a syscall only a later release knows
// is not checked.
//
// Lost rules: libseccomp before 2.6.2 can lose every rule of an
// architecture, which then applies the default to every call, where every
// rule on a syscall the architecture has is conditional and a rule on one it
// lacks is cheaper, as "socketcall" with one unconditional rule and "socket"
// with a conditional one are on x86_64. Allowlists, which name a syscall
// every covered architecture has in an unconditional rule, are not affected.
// Intersect and Union check every covered architecture of their result,
// erring safe for every supported release, and drop a rule on a syscall no
// covered architecture has, which is exact. Otherwise Intersect drops x32
// where it pairs with x86_64, x86_64 where x32 is native, the affected
// architecture where it is not native, and otherwise the listed
// architectures on which the offending syscall applies. It refuses with
// [ErrLostRules] where it can drop none: where the affected architecture is
// native and the syscall applies there too, or, where the native
// architecture is not known, the affected one is the last listed; such a node
// is checked as each architecture it may be, so the merge is refused more
// often. [Union] refuses such a result with ErrLostRules. [ValidateArtifact]
// and [ValidateStrict] report the shape as written (ErrLostRules); the
// merges settle the rules first and may accept it.
//
// 32-bit values: on a 32-bit architecture (x86, x32, arm, 32-bit and n32
// MIPS, ppc, s390, parisc, m68k, sh), libseccomp compares only the lower 32
// bits of each value and mask, so SCMP_CMP_EQ against 0x100000005 matches 5,
// and a SCMP_CMP_MASKED_EQ whose mask sets only upper bits matches every
// call. [ValidateArtifact] rejects such a condition ([ErrValueTooWide]).
// Where a merge result covers such an architecture and an input loads such a
// condition, Intersect drops the 32-bit architectures from the list, so
// their calls get SCMP_ACT_KILL while the rules keep their filters
// elsewhere; where the native architecture is a 32-bit one, it reads the
// affected syscalls as one unconditional rule instead, as it reads a rule
// set outside the safe shapes. Union always does the latter, in its
// direction. A rule whose every condition is a SCMP_CMP_MASKED_EQ with only
// upper mask bits holds for every call there, so where it comes before the
// syscall's unconditional rule it decides every call on a 32-bit
// architecture, and the unconditional one decides them elsewhere. Wherever a
// result covers a 32-bit architecture, both merges read such a syscall as
// its one unconditional rule: the more restrictive of the two for Intersect,
// the less restrictive for Union, even where Intersect keeps the 32-bit
// architectures.
//
// Multiplexers: on x86, 32-bit MIPS, ppc, ppc64, ppc64le, s390, s390x, m68k
// and sh, the socket and SysV IPC syscalls are also reached through
// socketcall(2) and ipc(2):
//
//	socketcall: accept, accept4, bind, connect, getpeername, getsockname,
//	            getsockopt, listen, recv, recvfrom, recvmmsg, recvmsg, send,
//	            sendmmsg, sendmsg, sendto, setsockopt, shutdown, socket,
//	            socketpair
//	ipc:        msgctl, msgget, msgrcv, msgsnd, semctl, semget, semop,
//	            semtimedop, shmat, shmctl, shmdt, shmget
//
// libseccomp adds every rule of such a syscall to the multiplexer a second
// time, with the condition on the first argument replaced by the sub-call
// number: socket ALLOW if a0 == 2 allows every socket(2) made through
// socketcall. The merges settle a result that would permit more
// (Intersect) or less (Union) on that path than an input, or that holds
// rules libseccomp may refuse there. Intersect drops the multiplexing
// architectures where the native architecture does not multiplex.
// Otherwise, and always for Union, the multiplexer and each affected
// syscall collapse to one unconditional rule in the merge direction, which
// also removes the filters from the direct syscall, except that an
// unconditional multiplexer rule differing from the default only in its
// errno is dropped where the multiplexed rules then decide every sub-call
// safely.
//
// Where the profile covers a multiplexing architecture, ValidateArtifact
// reports (ErrConflictingEntries) the rules of a multiplexed syscall whose
// copies libseccomp may refuse, and conditional rules of the multiplexer
// next to rules of a syscall it carries; [ValidateBaseline] reports what
// every merge settles in a profile, whatever the other inputs.
//
// Aliases: libseccomp gives switch_endian and sys_debug_setcontext one
// made-up number on every architecture but ppc, ppc64 and ppc64le, and
// reads their rules as one syscall's wherever it resolves both names to it:
// in the block of a covered architecture that keeps rules on syscalls it
// lacks (all but the multiplexing ones), and, where the native architecture
// lacks both, in a ppc, ppc64 or ppc64le block too. ValidateArtifact and
// ValidateStrict check the two as one syscall in such a filter, for the
// limits on one syscall and for conflicts, which libseccomp refuses with
// EEXIST or keeps only one of. On a node lacking both, the merges and
// [Diff] read them as one syscall, which is exact, and a result names both.
// On a PowerPC node, or one not known, the merges give both names what the
// two merge to where the rules a result holds for them would conflict in
// such a filter, and [ValidateBaseline] reports a baseline whose own rules
// on the two conflict so.
//
// Dropping an architecture from the list does not remove it from a filter
// where it is native, so Intersect is meant to run on the node that loads
// its result, and Diff compares the same two profiles differently depending
// on the native architecture. [Options] names the node: another node's
// architecture, the running program's, which the plain functions assume, or
// one that is not known, which the merges and validators read as possibly
// any architecture this package knows, since crun runs natively on some
// that no Go program does.
//
// # Flags
//
// Flags are merged by what they do, so that a merge never loosens a
// baseline. SECCOMP_FILTER_FLAG_SPEC_ALLOW disables a mitigation:
// intersection keeps it only if every input sets it, union if any does.
// SECCOMP_FILTER_FLAG_LOG adds audit logging: intersection keeps it if any
// input sets it, union only if every input does. SECCOMP_FILTER_FLAG_TSYNC,
// which runc ignores, is merged the same way, except that a result with a
// listener never carries it. SECCOMP_FILTER_FLAG_WAIT_KILLABLE_RECV comes
// from the input the listener comes from (see the Listener section).
// [Validate] rejects unknown flags.
//
// A nil list is not an empty one: it leaves the flags to the runtime, and
// runc since 1.2 and crun then set SECCOMP_FILTER_FLAG_SPEC_ALLOW, while a
// list that is set but empty sets no flag. The merges and [Diff] therefore
// read a nil list as one naming that flag. A result is nil again where the
// flag survives through an input that left it to the runtime and no other
// flag does; next to other flags the result names it. specs.LinuxSeccomp
// leaves an empty list out when marshaled, so write a result with [Marshal],
// which writes "flags": [] and keeps the flag off.
//
// # Listener
//
// ListenerPath, ListenerMetadata and SECCOMP_FILTER_FLAG_WAIT_KILLABLE_RECV
// come from the first input that sets a ListenerPath. A filter hands every
// notification to one listener, so inputs naming different listeners are
// refused with [ErrConflictingListener]: different non-empty listenerPath
// values, or one listenerPath with different listenerMetadata, even where
// only one input notifies. [Intersect] takes the listener from its first
// input only, the baseline the runtime trusts, since an artifact naming a
// node-local socket would choose who answers the workload's calls: a later
// input sets neither member or both exactly as the first input does, and is
// otherwise refused with ErrConflictingListener, also where the first input
// sets none. [Union], whose inputs are peers, takes the listener from
// whichever input sets one. [Validate] refuses a listenerMetadata without a
// listenerPath ([ErrListenerMetadataWithoutPath]), as the runtime-spec does.
//
// runc refuses a filter that notifies without a listenerPath, so [Validate]
// requires one from every profile that notifies ([ErrNotifyWithoutListener])
// and rejects SCMP_ACT_NOTIFY where runc refuses it outright, as the default
// action or on the write syscall ([ErrNotifyUnsupported]). Neither merge
// rewrites an action to keep the two together: a supervised syscall comes
// back supervised, and a merge refuses with ErrNotifyWithoutListener rather
// than emit a result that would notify into nothing.
//
// The kernel refuses SECCOMP_FILTER_FLAG_WAIT_KILLABLE_RECV without the flag
// that creates the listener, which runc sets only for a filter with a notify
// rule, and SECCOMP_FILTER_FLAG_TSYNC next to that flag, which crun sets for
// any listenerPath. Validate accepts both; [ValidateArtifact] and
// [ValidateStrict] refuse both, as they refuse every listener setting
// ([ErrListenerNotAllowed]). A merge result in which no entry notifies drops
// the listener flag, which a merge can produce even from inputs that notify,
// and a result with a listener drops TSYNC.
//
// # Single profiles
//
// Intersect(p) and Union(p) normalize a profile without merging: they reduce
// it to the rules a runtime loads from it, collapse syscalls whose
// conditional rules do not form a safe shape, in their direction, drop from
// the others each rule another rule with the same result covers, and settle
// the multiplexer path, as a merge does, but keep values above 32 bits,
// which mean on a 32-bit architecture what the profile means there. A syscall
// whose unconditional rule follows a rule that holds for every call on a
// 32-bit architecture the profile covers is read as the stricter or the
// looser of the two everywhere, which is not exact where the other one
// decides. Intersect(p) of a profile that notifies keeps the action and the
// listener.
//
// # Diff
//
// [Diff] compares syscall entries by the rules a runtime loads from them:
// entries equal to the profile default are ignored, an unconditional entry
// hides the conditional entries for its syscall and the first one wins,
// several conditions on one argument index are alternatives, conditions
// compare as libseccomp evaluates them, exact duplicates are dropped, and
// errno values and SCMP_ACT_KILL_THREAD are compared as the Errno values and
// Action order sections describe. Where the rules of a syscall form a safe
// shape, a rule that another rule with the same result covers is dropped
// too, as in a merge, such as "a0 == 8" next to "a0 < 16". The diff
// reports entries in that form, with EPERM spelled as unset. Rules are not
// rewritten beyond that: two rules with the same filter and different
// results both remain, as does a rule libseccomp's order never reaches
// outside the safe shapes. Architectures are compared with the native
// architecture implied on both sides.
//
// Where libseccomp keeps the profile order for two rules outside the safe
// shapes with different results (two conditions on one argument comparing
// the same value with SCMP_CMP_LT and SCMP_CMP_LE, with SCMP_CMP_GT and
// SCMP_CMP_GE, or with different masks), Diff also compares the order in
// which a runtime adds them, and reports a syscall where that differs with
// each side's rules in that order rather than sorted. It pairs the rules as
// for a 64-bit native architecture, so it may report an order a 32-bit
// node's filter does not depend on, and misses none it does depend on.
// Otherwise Diff does not check whether libseccomp accepts the rules: two
// profiles holding the same rules in a different order compare equal
// although libseccomp may refuse one of them.
//
// Diff(p, Intersect(p)) is equal only when every syscall of p is in a safe
// shape, nothing needed settling for a multiplexing architecture, no syscall
// has an unconditional rule after one that holds for every call on a 32-bit
// architecture p covers, no rule or architecture had to go to keep
// libseccomp from losing the rules of an architecture, and a profile setting
// SECCOMP_FILTER_FLAG_WAIT_KILLABLE_RECV keeps a rule that notifies.
// Otherwise it reports what the merge settled.
//
// Intersect(p, q), where q has default SCMP_ACT_ALLOW, no syscalls, the same
// architectures as p and the same SECCOMP_FILTER_FLAG_SPEC_ALLOW setting,
// compares equal to p when p is in safe shapes, needs no settling for a
// multiplexing architecture and, where it covers a 32-bit architecture,
// loads no value above 32 bits. The same holds for Union(p, q) with default
// SCMP_ACT_KILL_PROCESS, where q also needs the same SECCOMP_FILTER_FLAG_LOG
// setting. With other architectures, flags or defaults the result can differ
// from p.
//
// A syscall past the read budget (see the Cost bounds section) is listed by
// the two actions a collapse of it could pick rather than by its rules, and
// is reported as changed unless both profiles load the same rules for it
// from entries in the same order, even where the entries listed for it are
// the same on both sides.
//
// # Cost bounds
//
// The merges and [Diff] bound their work with internal budgets: on the
// product of the filtered rules two inputs hold for one syscall, for
// [Union] also on their sum, and on the rules one input loads in all, the
// read budget, which a small document can push into the millions since an
// entry loads a rule per name and, repeating an argument index, per
// condition. Past a budget a syscall is read or merged as the one
// unconditional rule a collapse picks, the most restrictive of its
// actions for intersection and the least restrictive for union, so
// [Intersect] still never permits more than any input and [Union] never
// less. Past the read budget, the syscalls of that input loading the most
// rules, ties broken by name, are read that way until the rest fit. For
// [Intersect], a side without filtered rules for a syscall adds no work, so
// a large artifact against a baseline that allows or denies the syscall
// unconditionally is merged precisely.
//
// The read budget keeps Diff, which validates nothing, and the merges
// linear in the size of a profile that skipped validation, but not cheap:
// Diff of such a profile allocates around 150 times its size in JSON.
// Validate an artifact with ValidateArtifact before anything else looks at
// it.
//
// Dropping the rules other rules with the same result cover, which Diff, a
// merge and a profile intersected alone all do, compares the rules of one
// syscall in pairs: of one result, only pairs on one argument index of
// which one is no equality, so a syscall of distinct equalities costs a
// pass, and otherwise every pair. A syscall can cost up to 512 squared such
// comparisons. Past 512 conditional rules nothing is dropped, so Diff and
// Intersect and Union of one profile stay linear in its syscalls with that
// factor for each, and Diff may report a covered rule there as a change.
//
// [ValidateArtifact] bounds an artifact so that it reports one too large to
// merge precisely instead of the merge silently collapsing a syscall, each
// limit with its own error matching [ErrArtifactTooLarge]:
// [MaxArtifactEntriesPerSyscall] and [MaxArtifactRulesPerSyscall] bound one
// syscall, [MaxArtifactNamesPerEntry] the names of one entry,
// [MaxArtifactRules] the rules of the whole profile and [MaxArtifactEntries]
// its entries, which only entries loading no rule can exceed. Rules are
// counted the way runtimes add them: one per entry naming the syscall, one
// per condition for an entry repeating an argument index, none for an entry
// equal to the default, so the rule limits also bound the conditions of
// such an entry. The limits on the whole profile are checked first and
// reported on their own. An accepted artifact is read rule by rule, and
// merged precisely against a baseline with up to a dozen filtered rules for
// a syscall; a baseline with more, or a union with several hundred filtered
// rules for one syscall, can still collapse it. [Validate] enforces none of
// the limits, since a runtime loads a profile past them.
//
// # Conflicting rules
//
// [ValidateArtifact] and [ValidateStrict] reject rules for one syscall with
// different results ([ErrConflictingEntries]) when the filter of one is
// equal to or wider than the other's (its conditions are a subset, which
// includes an unconditional rule), and when the conditional rules do not
// form a safe shape. This covers every case in which libseccomp refuses a
// rule with EEXIST, which runc and crun report as a failure to load the
// profile, or keeps only one of the rules, whatever the order of the
// entries. Filters are also compared with every value truncated to 32
// bits, as on a 32-bit architecture. Rules are counted and compared the way
// runtimes add them, and these checks only run when [Validate] passes.
//
// A profile that passes may still hold rules sharing one result that
// libseccomp evaluates in its own order, miscompiles, or never finishes
// adding, and on a multiplexing architecture rules libseccomp refuses (see
// the Architectures section). The merges read such rules conservatively and
// never emit them, so runtimes should load the merge result rather than the
// artifact itself.
//
// # Concurrency
//
// Every exported function is safe to call from several goroutines at once.
// The functions hold no state between calls and never modify their
// arguments, except [UnmarshalStrict], which replaces the profile it is
// given with the decoded one when decoding succeeds, so concurrent calls
// only need their profiles not to be written to at the same time from
// elsewhere.
//
// What [Intersect], [Union] and [Diff] return shares no memory with their
// arguments, also for a single input, so it is the caller's to keep or
// modify without affecting the inputs, and modifying an input later leaves
// the result as it was.
package seccomp
