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
	"maps"
	"math"
	"slices"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// libseccomp 2.5.2, 2.5.4, 2.5.5, 2.6.0 and 2.6.1 emit the instruction that
// loads the syscall number in the code of the syscall an architecture's
// block tests first, and leave that code out when the syscall is one the
// architecture lacks, which libseccomp keeps in the block under a number it
// makes up. Every rule of the block then compares against a value that is no
// syscall number, and every call of the architecture gets the default
// action. 2.6.2 loads the number in the first block it emits
// (GHSA-q75f-27j4-2h7j). The merges keep their results out of that shape
// for the earlier releases (see settleAbsentSyscalls and the Architectures
// section of the package documentation).
//
// A block tests its syscalls in the order of how many comparisons their
// rules compile to, fewest first, and a syscall the architecture has wins a
// tie against one it lacks.
//
// x86_64 and x32 share an AUDIT_ARCH value, so their calls are told apart
// by the syscall number, and libseccomp compiles their blocks as one. A
// filter covering only one of them loads the number before any syscall is
// tested, to refuse the other's calls. A filter covering both loads it in
// the code of the first syscall tested, so a block that tests a syscall its
// architecture lacks first also loses the other block's rules: the other's
// calls are no longer told apart and get the default action, even where
// the first block holds no rule on a syscall its own architecture has. No
// other pair of architectures shares an AUDIT_ARCH value.
//
// All of it was read from the programs libseccomp 2.6.1 compiles, and the
// libseccomp tests of this package keep checking it on every release the
// placement table below combines.

// syscallPlacement is where libseccomp places a rule on one syscall, per
// architecture, as bits of syscallArchitectures: under the number of the
// syscall (direct), under a number it makes up for a syscall the
// architecture lacks (kept), and as a copy on the multiplexer socketcall(2)
// or ipc(2) (viaMux). A non-multiplexing architecture keeps every syscall
// it lacks; a multiplexing one keeps only a multiplexed syscall it has no
// number for and drops the rest when the rule is added.
//
// syscallPlacements combines libseccomp 2.5.2, 2.5.4, 2.5.5, 2.6.0, 2.6.1
// and 2.6.2, the releases runtimes link, which disagree where one knows a
// syscall or a number another does not. Each decision reads the combination
// that errs safe for all of them:
//
//   - which syscall a block tests first is decided among those direct and
//     viaMux name, which hold where every release places the rule so: a
//     syscall one release lacks then cannot hide a cheaper absent one;
//   - the syscalls that can come first are those kept names, which holds
//     where any release keeps the rule;
//   - whether a block has rules at stake, its own or its partner's, reads
//     reach, which holds where any release places the rule under its
//     number or on the multiplexer: a rule only 2.6.1 knows is lost on a
//     node that links 2.6.1, and one 2.5.2 does not know is not loaded at
//     all on a node that links 2.5.2;
//   - whether a rule applies to no call of the filter, and so can go, and
//     which architectures an offending syscall applies on, read reach too.
//
// It is generated from the syscalls.csv each release ships in its source
// tree, so that it does not depend on the architecture it is generated on:
// make verify-syscalls fetches the files, regenerates syscalls.go with
// SPM_SYSCALL_CSV_DIR naming their directory, and fails when the result
// differs from the file checked in.
type syscallPlacement struct {
	direct, kept, viaMux, reach uint32
}

// sharedAuditArch pairs the architectures that share an AUDIT_ARCH value,
// which lose rules only together (see above).
var sharedAuditArch = map[specs.Arch]specs.Arch{
	specs.ArchX86_64: specs.ArchX32,
	specs.ArchX32:    specs.ArchX86_64,
}

// syscallArchitectures are the architectures syscallPlacements describes,
// in the order of its bits.
var syscallArchitectures = [...]specs.Arch{
	specs.ArchX86, specs.ArchX86_64, specs.ArchX32,
	specs.ArchARM, specs.ArchAARCH64,
	specs.ArchMIPS, specs.ArchMIPS64, specs.ArchMIPS64N32,
	specs.ArchMIPSEL, specs.ArchMIPSEL64, specs.ArchMIPSEL64N32,
	specs.ArchPPC, specs.ArchPPC64, specs.ArchPPC64LE,
	specs.ArchS390, specs.ArchS390X,
	specs.ArchPARISC, specs.ArchPARISC64,
	specs.ArchRISCV64, specs.ArchLOONGARCH64,
	specs.ArchM68K, specs.ArchSH, specs.ArchSHEB,
}

// archBit returns the bit of arch in syscallPlacements, or 0 for an
// architecture the table does not describe.
func archBit(arch specs.Arch) uint32 {
	for idx, candidate := range syscallArchitectures {
		if candidate == arch {
			return 1 << idx
		}
	}

	return 0
}

// archMask returns the bits of the architectures in syscallPlacements.
func archMask(archs []specs.Arch) uint32 {
	var mask uint32

	for _, arch := range archs {
		mask |= archBit(arch)
	}

	return mask
}

// coveredArchitectures returns the architectures a filter loaded from a
// profile listing archs covers: those it lists, and native, which runtimes
// always add. native is empty where it is not known.
func coveredArchitectures(archs []specs.Arch, native specs.Arch) []specs.Arch {
	covered := slices.Clone(archs)
	if native != "" {
		covered = append(covered, native)
	}

	slices.Sort(covered)

	return slices.Compact(covered)
}

// partnerIn returns the architecture covered sharing the AUDIT_ARCH value
// of arch, and whether there is one.
func partnerIn(arch specs.Arch, covered []specs.Arch) (specs.Arch, bool) {
	partner, paired := sharedAuditArch[arch]

	return partner, paired && slices.Contains(covered, partner)
}

// losesRules reports whether the block of arch in a filter covering covered
// can lose rules to a syscall the architecture lacks: always, except for an
// architecture whose AUDIT_ARCH value it shares with none of the others.
func losesRules(arch specs.Arch, covered []specs.Arch) bool {
	if _, shares := sharedAuditArch[arch]; shares {
		_, paired := partnerIn(arch, covered)

		return paired
	}

	return true
}

// The comparisons libseccomp compiles a condition to where it compares 64
// bits (see comparisonCost).
const (
	wideEqualityCost = 2
	wideRangeCost    = 3
)

// comparisonCost returns how many comparisons libseccomp compiles one
// condition to on an architecture. Where it compares 64 bits, an equality,
// inequality or masked comparison tests the upper and the lower half, and a
// range comparison needs a third to order them. Where it compares 32 bits,
// every condition is one comparison, except a masked comparison whose mask
// is empty in the lower half, which it drops.
func comparisonCost(arg specs.LinuxSeccompArg, arch specs.Arch) int {
	if narrowArchitectures[arch] {
		if arg.Op == specs.OpMaskedEqual && arg.Value&lower32 == 0 {
			return 0
		}

		return 1
	}

	if arg.Op == specs.OpEqualTo || arg.Op == specs.OpNotEqual || arg.Op == specs.OpMaskedEqual {
		return wideEqualityCost
	}

	// The range comparisons, and an operator Validate refuses, which costs
	// no less.
	return wideRangeCost
}

// comparisonCount bounds how many comparisons libseccomp compiles the rules
// of one syscall in one block to. A rule without comparisons makes the
// syscall a leaf with none. Otherwise rules sharing a prefix share its
// comparisons, so their sum is an upper bound, and libseccomp may drop a
// rule below a shorter one, so the cheapest rule is a lower one.
type comparisonCount struct {
	leaf     bool
	sum      int
	cheapest int
	rules    int
}

func (c *comparisonCount) add(args []specs.LinuxSeccompArg, arch specs.Arch) {
	cost := 0
	for _, arg := range args {
		cost += comparisonCost(arg, arch)
	}

	if cost == 0 {
		c.leaf = true
	}

	c.sum += cost

	if c.rules == 0 || cost < c.cheapest {
		c.cheapest = cost
	}

	c.rules++
}

func (c *comparisonCount) upperBound() int {
	if c.leaf {
		return 0
	}

	return c.sum
}

func (c *comparisonCount) lowerBound() int {
	if c.leaf {
		return 0
	}

	return c.cheapest
}

// multiplexedArgs returns the conditions of the copy libseccomp adds on the
// multiplexer for a rule on a multiplexed syscall: the first argument
// matches the sub-call, and the conditions on the others are kept.
func multiplexedArgs(args []specs.LinuxSeccompArg) []specs.LinuxSeccompArg {
	copied := []specs.LinuxSeccompArg{{Index: 0, Value: 0, ValueTwo: 0, Op: specs.OpEqualTo}}

	for _, arg := range args {
		if arg.Index != 0 {
			copied = append(copied, arg)
		}
	}

	return copied
}

// absentSettler carries settleAbsentSyscalls through one merge result. The
// tally of a block depends on the rules alone, not on which architectures
// the filter covers or which is native, so each block is tallied once and
// kept across the natives tried and the steps taken. Dropping an
// architecture leaves the tallies as they are; dropping rules updates them
// (see dropRules).
type absentSettler struct {
	m      ruleMerger
	result *specs.LinuxSeccomp
	blocks map[specs.Arch]*blockTally
	// asWritten keeps the rules that apply to no call of the filter, which
	// a merge drops: a runtime loading a profile as written keeps them too,
	// under a number libseccomp makes up, and they are what can lose the
	// rules of a block. The validators judge a profile that way.
	asWritten bool
	// tallied and steps count the blocks tallied and the steps taken, so
	// that a test can check that the work does not grow with their
	// product.
	tallied, steps int
}

func newAbsentSettler(m ruleMerger, result *specs.LinuxSeccomp) *absentSettler {
	return &absentSettler{
		m:      m,
		result: result,
		blocks: make(map[specs.Arch]*blockTally),
	}
}

// block returns the tally of the block of arch, tallying it the first time.
func (s *absentSettler) block(arch specs.Arch) *blockTally {
	tally, ok := s.blocks[arch]
	if !ok {
		tally = blockCounts(s.result.Syscalls, arch)
		s.blocks[arch] = tally
		s.tallied++
	}

	return tally
}

// absentFirst returns the syscalls the block of arch, in a filter covering
// covered, may test before every syscall it has, which libseccomp leaves
// out together with the load of the syscall number: syscalls it keeps for
// lacking them whose rules may compile to fewer comparisons than those of
// every syscall it has, or all of them where it has none. It returns nil
// when no rule is at stake: when the block holds no rule on a syscall its
// architecture has and shares its AUDIT_ARCH value with no covered
// architecture whose block does. Names libseccomp does not know are left
// out: a runtime cannot add a rule for them.
//
// The tally reads every entry as placing its rules, which holds for a
// merge result: the merge drops every entry with the result of the
// default, which a runtime would skip.
func (s *absentSettler) absentFirst(arch specs.Arch, covered []specs.Arch) []string {
	block := s.block(arch)
	if len(block.absent) == 0 || !block.atStake && !s.partnerHasRules(arch, covered) {
		return nil
	}

	fewest := math.MaxInt
	for _, count := range block.present {
		fewest = min(fewest, count.upperBound())
	}

	var first []string

	for _, name := range slices.Sorted(maps.Keys(block.absent)) {
		if block.absent[name].lowerBound() < fewest {
			first = append(first, name)
		}
	}

	return first
}

// partnerHasRules reports whether the block of the covered architecture
// sharing the AUDIT_ARCH value of arch holds a rule on a syscall its
// architecture has, which a block of arch that loses the load of the
// syscall number loses as well.
func (s *absentSettler) partnerHasRules(arch specs.Arch, covered []specs.Arch) bool {
	partner, paired := partnerIn(arch, covered)
	if !paired {
		return false
	}

	return s.block(partner).atStake
}

// blockCounts returns the comparisons of the syscalls the block of arch
// holds rules on, split into those every release places under their number
// or on the multiplexer, which decide what the block tests first, and those
// some release keeps for lacking them, and whether some release places a
// rule on a syscall the architecture has, which the block then has at
// stake (see syscallPlacement).
func blockCounts(syscalls []specs.LinuxSyscall, arch specs.Arch) *blockTally {
	tally := &blockTally{
		arch:    arch,
		bit:     archBit(arch),
		present: make(map[string]*comparisonCount),
		absent:  make(map[string]*comparisonCount),
	}

	for idx := range syscalls {
		tally.entry(&syscalls[idx])
	}

	return tally
}

// blockTally collects blockCounts for the block of one architecture.
type blockTally struct {
	arch            specs.Arch
	bit             uint32
	present, absent map[string]*comparisonCount
	atStake         bool
}

// entry records the rules one entry places in the block. Its conditions
// are read only for a syscall the block places: many entries of a result
// name a syscall libseccomp does not know or places elsewhere.
func (b *blockTally) entry(entry *specs.LinuxSyscall) {
	var args []specs.LinuxSeccompArg

	loaded := false

	for _, name := range entry.Names {
		placed, known := syscallPlacements[name]
		if !known || (placed.direct|placed.kept|placed.viaMux|placed.reach)&b.bit == 0 {
			continue
		}

		b.atStake = b.atStake || placed.reach&b.bit != 0

		if !loaded {
			args, loaded = loadedArgs(entry.Args), true
		}

		if placed.direct&b.bit != 0 {
			b.record(b.present, name, args)
		}

		if placed.kept&b.bit != 0 {
			b.record(b.absent, name, args)
		}

		if placed.viaMux&b.bit != 0 {
			b.record(b.present, multiplexedSyscalls[name], multiplexedArgs(args))
		}
	}
}

func (b *blockTally) record(
	side map[string]*comparisonCount, name string, args []specs.LinuxSeccompArg,
) {
	count, ok := side[name]
	if !ok {
		count = &comparisonCount{}
		side[name] = count
	}

	count.add(args, b.arch)
}

// losingBlock returns the first covered architecture whose block can lose
// its rules, and the syscalls it may test first, or the empty Arch.
func (s *absentSettler) losingBlock(covered []specs.Arch) (specs.Arch, []string) {
	for _, arch := range covered {
		if !losesRules(arch, covered) {
			continue
		}

		if first := s.absentFirst(arch, covered); len(first) > 0 {
			return arch, first
		}
	}

	return "", nil
}

// dropRules drops the entries naming a syscall in gone and brings the
// tallies up to date. A block in which a dropped entry placed nothing but
// rules kept for lacking a gone syscall loses those from its tally, which
// is exact, since every entry naming that syscall goes; any other block an
// entry placed something in is tallied afresh when it is next needed.
//
// Every entry of a merge result names one syscall until regroupSyscalls
// joins them, but an entry naming several is read whole all the same: the
// names that do not go mark the blocks they are kept in stale too, since
// the entry takes their rules with it. An entry naming none places nothing
// and stays.
func (s *absentSettler) dropRules(gone map[string]bool) {
	var stale uint32

	s.result.Syscalls = slices.DeleteFunc(s.result.Syscalls, func(entry specs.LinuxSyscall) bool {
		if !slices.ContainsFunc(entry.Names, func(name string) bool { return gone[name] }) {
			return false
		}

		for _, name := range entry.Names {
			placed := syscallPlacements[name]
			stale |= placed.direct | placed.viaMux | placed.reach

			if !gone[name] {
				stale |= placed.kept
			}
		}

		return true
	})

	for arch, tally := range s.blocks {
		if stale&archBit(arch) != 0 {
			delete(s.blocks, arch)

			continue
		}

		for name := range gone {
			delete(tally.absent, name)
		}
	}
}

// settleAbsentSyscalls keeps a merge result out of the shape in which
// libseccomp loses the rules of an architecture (see absentFirst). While a
// covered architecture's block can lose its rules:
//
//   - rules on an offending syscall that no covered architecture has are
//     dropped, which is exact: they apply to no call of the filter. Where
//     the native architecture is not known, the runtime adds one this
//     cannot see, so a rule is dropped only where no architecture has the
//     syscall;
//   - otherwise intersection drops an architecture from the list, as it
//     does for the 32-bit and multiplexing architectures, and libseccomp
//     then answers its calls with SCMP_ACT_KILL: the losing architecture
//     where it is not native, x32 where it pairs with x86_64 (x86_64
//     where x32 is named native), and otherwise every listed architecture
//     on which an offending syscall applies, which leaves it applying
//     nowhere;
//   - where no architecture can be dropped, and always for union, which
//     cannot drop an architecture an input covers, the merge is refused
//     with ErrLostRules. Where the native architecture is not known, every
//     architecture this package knows is tried as native, only x32 can be
//     dropped (see droppedFor), and dropping the last listed architecture
//     counts as none: the empty list would leave the filter to whatever
//     architecture loads it.
//
// Each step removes rules or architectures, so the loop ends.
func (m ruleMerger) settleAbsentSyscalls(result *specs.LinuxSeccomp) error {
	return newAbsentSettler(m, result).settle()
}

func (s *absentSettler) settle() error {
	for {
		s.steps++

		arch, offending, covered := s.losingBlockOf()
		if arch == "" {
			return nil
		}

		if !s.asWritten {
			gone := s.m.appliesNowhere(offending, covered)
			if len(gone) > 0 {
				s.dropRules(gone)

				continue
			}
		}

		listed := s.result.Architectures
		remaining := withoutAny(listed, s.m.droppedFor(arch, offending, covered))

		if !s.m.intersect || len(remaining) == len(listed) ||
			len(remaining) == 0 && s.m.native == "" {
			return s.lostRules(arch, offending[0])
		}

		s.result.Architectures = remaining
	}
}

// lostRules reports the block of arch losing its rules to the offending
// syscall. Where the native architecture is not known, the block may be one
// the list does not name, which the runtime adds as native, and the message
// says so.
func (s *absentSettler) lostRules(arch specs.Arch, offending string) error {
	possible := ""
	if s.m.native == "" && !slices.Contains(s.result.Architectures, arch) {
		possible = ", a possible native architecture"
	}

	return fmt.Errorf(
		"%s%s: %w (it would test %s first, which it lacks)",
		arch, possible, ErrLostRules, offending,
	)
}

// losingBlockOf returns the architecture whose block can lose its rules in
// the filter a runtime loads the result into, the syscalls it may test
// first, and the architectures that filter covers, or the empty Arch. Where
// the native architecture is not known, the runtime may add any of
// possibleNatives, so each of them is tried in turn.
func (s *absentSettler) losingBlockOf() (specs.Arch, []string, []specs.Arch) {
	natives := []specs.Arch{s.m.native}
	if s.m.native == "" {
		natives = possibleNatives
	}

	for _, native := range natives {
		covered := coveredArchitectures(s.result.Architectures, native)

		arch, offending := s.losingBlock(covered)
		if arch != "" {
			return arch, offending, covered
		}
	}

	return "", nil, nil
}

// possibleNatives are the architectures a runtime may add to a filter as
// its native one, sorted: every architecture this package knows. runc only
// runs where Go does, but crun is written in C and libseccomp resolves a
// native architecture on every platform it supports, x32, MIPS n32, ppc,
// s390, sh, m68k and parisc included, which no Go port targets.
var possibleNatives = slices.Sorted(maps.Keys(byteOrders))

// appliesNowhere returns the offending syscalls a rule on applies to no
// call of the filter: no covered architecture has them, or where the native
// architecture is not known, no architecture at all.
func (m ruleMerger) appliesNowhere(offending []string, covered []specs.Arch) map[string]bool {
	coveredMask := archMask(covered)
	if m.native == "" {
		coveredMask = math.MaxUint32
	}

	gone := make(map[string]bool)

	for _, name := range offending {
		if syscallPlacements[name].reach&coveredMask == 0 {
			gone[name] = true
		}
	}

	return gone
}

// droppedFor returns the architectures intersection drops to keep the block
// of arch from losing its rules to the offending syscalls (see
// settleAbsentSyscalls).
//
// Where the native architecture is not known, any architecture may be
// native, and dropping it from the list would not remove it from the
// filter, so only x32 is offered, for the natives on which dropping it
// helps; settle tries every possible native in turn, and one on which x32
// is native finds the list unchanged and refuses the merge.
func (m ruleMerger) droppedFor(
	arch specs.Arch, offending []string, covered []specs.Arch,
) map[specs.Arch]bool {
	if _, shares := sharedAuditArch[arch]; shares {
		// x86_64 and x32 lose rules only together. Of the two, x32 is the
		// one few processes use, but a node may be natively x32: the
		// runtime then adds it back, so it is x86_64 that goes.
		if m.native == specs.ArchX32 {
			return map[specs.Arch]bool{specs.ArchX86_64: true}
		}

		return map[specs.Arch]bool{specs.ArchX32: true}
	}

	if m.native == "" {
		return nil
	}

	if arch != m.native {
		return map[specs.Arch]bool{arch: true}
	}

	dropped := make(map[specs.Arch]bool)

	for _, name := range offending {
		for _, candidate := range covered {
			if candidate != m.native && syscallPlacements[name].reach&archBit(candidate) != 0 {
				dropped[candidate] = true
			}
		}
	}

	return dropped
}
