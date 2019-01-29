// Package psgo is a ps (1) AIX-format compatible golang library extended with
// various descriptors useful for displaying container-related data.
//
// The idea behind the library is to provide an easy to use way of extracting
// process-related data, just as ps (1) does. The problem when using ps (1) is
// that the ps format strings split columns with whitespaces, making the output
// nearly impossible to parse. It also adds some jitter as we have to fork and
// execute ps either in the container or filter the output afterwards, further
// limiting applicability.
//
// Please visit https://github.com/containers/psgo for further details about
// supported format descriptors and to see some usage examples.
package psgo

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/containers/psgo/internal/capabilities"
	"github.com/containers/psgo/internal/dev"
	"github.com/containers/psgo/internal/proc"
	"github.com/containers/psgo/internal/process"
	"github.com/containers/psgo/internal/types"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// processFunc is used to map a given aixFormatDescriptor to a corresponding
// function extracting the desired data from a process.
type processFunc func(*process.Process) (string, error)

// aixFormatDescriptor as mentioned in the ps(1) manpage.  A given descriptor
// can either be specified via its code (e.g., "%C") or its normal representation
// (e.g., "pcpu") and will be printed under its corresponding header (e.g, "%CPU").
type aixFormatDescriptor struct {
	// code descriptor in the short form (e.g., "%C").
	code string
	// normal descriptor in the long form (e.g., "pcpu").
	normal string
	// header of the descriptor (e.g., "%CPU").
	header string
	// onHost controls if data of the corresponding host processes will be
	// extracted as well.
	onHost bool
	// procFN points to the corresponding method to etract the desired data.
	procFn processFunc
}

// translateDescriptors parses the descriptors and returns a correspodning slice of
// aixFormatDescriptors.  Descriptors can be specified in the normal and in the
// code form (if supported).  If the descriptors slice is empty, the
// `DefaultDescriptors` is used.
func translateDescriptors(descriptors []string) ([]aixFormatDescriptor, error) {
	if len(descriptors) == 0 {
		descriptors = DefaultDescriptors
	}

	formatDescriptors := []aixFormatDescriptor{}
	for _, d := range descriptors {
		d = strings.TrimSpace(d)
		found := false
		for _, aix := range aixFormatDescriptors {
			if d == aix.code || d == aix.normal {
				formatDescriptors = append(formatDescriptors, aix)
				found = true
			}
		}
		if !found {
			return nil, errors.Wrapf(ErrUnkownDescriptor, "'%s'", d)
		}
	}

	return formatDescriptors, nil
}

var (
	// DefaultDescriptors is the `ps -ef` compatible default format.
	DefaultDescriptors = []string{"user", "pid", "ppid", "pcpu", "etime", "tty", "time", "args"}

	// ErrUnkownDescriptor is returned when an unknown descriptor is parsed.
	ErrUnkownDescriptor = errors.New("unknown descriptor")

	// hostProcesses are the processes on the host.  It should only be used
	// in the context of containers and is meant to display data of the
	// container processes from the host's (i.e., calling process) view.
	// Currently, all host processes contain only the required data from
	// /proc/$pid/status.
	hostProcesses []*process.Process

	aixFormatDescriptors = []aixFormatDescriptor{
		{
			code:   "%C",
			normal: "pcpu",
			header: "%CPU",
			procFn: processPCPU,
		},
		{
			code:   "%G",
			normal: "group",
			header: "GROUP",
			procFn: processGROUP,
		},
		{
			code:   "%P",
			normal: "ppid",
			header: "PPID",
			procFn: processPPID,
		},
		{
			code:   "%U",
			normal: "user",
			header: "USER",
			procFn: processUSER,
		},
		{
			code:   "%a",
			normal: "args",
			header: "COMMAND",
			procFn: processARGS,
		},
		{
			code:   "%c",
			normal: "comm",
			header: "COMMAND",
			procFn: processCOMM,
		},
		{
			code:   "%g",
			normal: "rgroup",
			header: "RGROUP",
			procFn: processRGROUP,
		},
		{
			code:   "%n",
			normal: "nice",
			header: "NI",
			procFn: processNICE,
		},
		{
			code:   "%p",
			normal: "pid",
			header: "PID",
			procFn: processPID,
		},
		{
			code:   "%r",
			normal: "pgid",
			header: "PGID",
			procFn: processPGID,
		},
		{
			code:   "%t",
			normal: "etime",
			header: "ELAPSED",
			procFn: processETIME,
		},
		{
			code:   "%u",
			normal: "ruser",
			header: "RUSER",
			procFn: processRUSER,
		},
		{
			code:   "%x",
			normal: "time",
			header: "TIME",
			procFn: processTIME,
		},
		{
			code:   "%y",
			normal: "tty",
			header: "TTY",
			procFn: processTTY,
		},
		{
			code:   "%z",
			normal: "vsz",
			header: "VSZ",
			procFn: processVSZ,
		},
		{
			normal: "capamb",
			header: "AMBIENT CAPS",
			procFn: processCAPAMB,
		},
		{
			normal: "capinh",
			header: "INHERITED CAPS",
			procFn: processCAPINH,
		},
		{
			normal: "capprm",
			header: "PERMITTED CAPS",
			procFn: processCAPPRM,
		},
		{
			normal: "capeff",
			header: "EFFECTIVE CAPS",
			procFn: processCAPEFF,
		},
		{
			normal: "capbnd",
			header: "BOUNDING CAPS",
			procFn: processCAPBND,
		},
		{
			normal: "seccomp",
			header: "SECCOMP",
			procFn: processSECCOMP,
		},
		{
			normal: "label",
			header: "LABEL",
			procFn: processLABEL,
		},
		{
			normal: "hpid",
			header: "HPID",
			onHost: true,
			procFn: processHPID,
		},
		{
			normal: "huser",
			header: "HUSER",
			onHost: true,
			procFn: processHUSER,
		},
		{
			normal: "hgroup",
			header: "HGROUP",
			onHost: true,
			procFn: processHGROUP,
		},
		{
			normal: "state",
			header: "STATE",
			procFn: processState,
		},
	}
)

// ListDescriptors returns a string slice of all supported AIX format
// descriptors in the normal form.
func ListDescriptors() (list []string) {
	for _, d := range aixFormatDescriptors {
		list = append(list, d.normal)
	}
	sort.Strings(list)
	return
}

// JoinNamespaceAndProcessInfo has the same semantics as ProcessInfo but joins
// the mount namespace of the specified pid before extracting data from `/proc`.
func JoinNamespaceAndProcessInfo(pid string, descriptors []string) ([][]string, error) {
	var (
		data    [][]string
		dataErr error
		wg      sync.WaitGroup
	)

	aixDescriptors, err := translateDescriptors(descriptors)
	if err != nil {
		return nil, err
	}

	// extract data from host processes only on-demand / when at least one
	// of the specified descriptors requires host data
	for _, d := range aixDescriptors {
		if d.onHost {
			setHostProcesses(pid)
			break
		}
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		runtime.LockOSThread()

		// extract user namespaces prior to joining the mount namespace
		currentUserNs, err := proc.ParseUserNamespace("self")
		if err != nil {
			dataErr = errors.Wrapf(err, "error determining user namespace")
			return
		}

		pidUserNs, err := proc.ParseUserNamespace(pid)
		if err != nil {
			dataErr = errors.Wrapf(err, "error determining user namespace of PID %s", pid)
		}

		// join the mount namespace of pid
		fd, err := os.Open(fmt.Sprintf("/proc/%s/ns/mnt", pid))
		if err != nil {
			dataErr = err
			return
		}
		defer fd.Close()

		// create a new mountns on the current thread
		if err = unix.Unshare(unix.CLONE_NEWNS); err != nil {
			dataErr = err
			return
		}
		unix.Setns(int(fd.Fd()), unix.CLONE_NEWNS)

		// extract all pids mentioned in pid's mount namespace
		pids, err := proc.GetPIDs()
		if err != nil {
			dataErr = err
			return
		}

		ctx := types.PsContext{
			// join the user NS if the pid's user NS is different
			// to the caller's user NS.
			JoinUserNS: currentUserNs != pidUserNs,
		}
		processes, err := process.FromPIDs(&ctx, pids)
		if err != nil {
			dataErr = err
			return
		}

		data, dataErr = processDescriptors(aixDescriptors, processes)
	}()
	wg.Wait()

	return data, dataErr
}

// JoinNamespaceAndProcessInfoByPids has similar semantics to
// JoinNamespaceAndProcessInfo and avoids duplicate entries by joining a giving
// PID namepsace only once.
func JoinNamespaceAndProcessInfoByPids(pids []string, descriptors []string) ([][]string, error) {
	// Extracting data from processes that share the same PID namespace
	// would yield duplicate results.  Avoid that by extracting data only
	// from the first process in `pids` from a given PID namespace.
	// `nsMap` is used for quick lookups if a given PID namespace is
	// already covered, `pidList` is used to preserve the order which is
	// not guaranteed by nondeterministic maps in golang.
	nsMap := make(map[string]bool)
	pidList := []string{}
	for _, pid := range pids {
		ns, err := proc.ParsePIDNamespace(pid)
		if err != nil {
			if os.IsNotExist(err) {
				// catch race conditions
				continue
			}
			return nil, errors.Wrapf(err, "error extracing PID namespace")
		}
		if _, exists := nsMap[ns]; !exists {
			nsMap[ns] = true
			pidList = append(pidList, pid)
		}
	}

	data := [][]string{}
	for i, pid := range pidList {
		pidData, err := JoinNamespaceAndProcessInfo(pid, descriptors)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			data = append(data, pidData[0])
		}
		data = append(data, pidData[1:]...)
	}

	return data, nil
}

// ProcessInfo returns the process information of all processes in the current
// mount namespace. The input format must be a comma-separated list of
// supported AIX format descriptors.  If the input string is empty, the
// `DefaultDescriptors` is used.
// The return value is an array of tab-separated strings, to easily use the
// output for column-based formatting (e.g., with the `text/tabwriter` package).
func ProcessInfo(descriptors []string) ([][]string, error) {
	pids, err := proc.GetPIDs()
	if err != nil {
		return nil, err
	}

	return ProcessInfoByPids(pids, descriptors)
}

// ProcessInfoByPids is like ProcessInfo, but the process information returned
// is limited to a list of user specified PIDs.
func ProcessInfoByPids(pids []string, descriptors []string) ([][]string, error) {
	aixDescriptors, err := translateDescriptors(descriptors)
	if err != nil {
		return nil, err
	}

	ctx := types.PsContext{JoinUserNS: false}
	processes, err := process.FromPIDs(&ctx, pids)
	if err != nil {
		return nil, err
	}

	return processDescriptors(aixDescriptors, processes)
}

// setHostProcesses sets `hostProcesses`.
func setHostProcesses(pid string) error {
	// get processes
	pids, err := proc.GetPIDsFromCgroup(pid)
	if err != nil {
		return err
	}

	ctx := types.PsContext{JoinUserNS: false}
	processes, err := process.FromPIDs(&ctx, pids)
	if err != nil {
		return err
	}

	// set the additional host data
	for _, p := range processes {
		if err := p.SetHostData(); err != nil {
			return err
		}
	}

	hostProcesses = processes
	return nil
}

// processDescriptors calls each `procFn` of all formatDescriptors on each
// process and returns an array of tab-separated strings.
func processDescriptors(formatDescriptors []aixFormatDescriptor, processes []*process.Process) ([][]string, error) {
	data := [][]string{}
	// create header
	header := []string{}
	for _, desc := range formatDescriptors {
		header = append(header, desc.header)
	}
	data = append(data, header)

	// dispatch all descriptor functions on each process
	for _, proc := range processes {
		pData := []string{}
		for _, desc := range formatDescriptors {
			dataStr, err := desc.procFn(proc)
			if err != nil {
				return nil, err
			}
			pData = append(pData, dataStr)
		}
		data = append(data, pData)
	}

	return data, nil
}

// findHostProcess returns the corresponding process from `hostProcesses` or
// nil if non is found.
func findHostProcess(p *process.Process) *process.Process {
	for _, hp := range hostProcesses {
		// We expect the host process to be in another namespace, so
		// /proc/$pid/status.NSpid must have at least two entries.
		if len(hp.Status.NSpid) < 2 {
			continue
		}
		// The process' PID must match the one in the NS of the host
		// process and both must share the same pid NS.
		if p.Pid == hp.Status.NSpid[1] && p.PidNS == hp.PidNS {
			return hp
		}
	}
	return nil
}

// processGROUP returns the effective group ID of the process.  This will be
// the textual group ID, if it can be optained, or a decimal representation
// otherwise.
func processGROUP(p *process.Process) (string, error) {
	return process.LookupGID(p.Status.Gids[1])
}

// processRGROUP returns the real group ID of the process.  This will be
// the textual group ID, if it can be optained, or a decimal representation
// otherwise.
func processRGROUP(p *process.Process) (string, error) {
	return process.LookupGID(p.Status.Gids[0])
}

// processPPID returns the parent process ID of process p.
func processPPID(p *process.Process) (string, error) {
	return p.Status.PPid, nil
}

// processUSER returns the effective user name of the process.  This will be
// the textual user ID, if it can be optained, or a decimal representation
// otherwise.
func processUSER(p *process.Process) (string, error) {
	return process.LookupUID(p.Status.Uids[1])
}

// processRUSER returns the effective user name of the process.  This will be
// the textual user ID, if it can be optained, or a decimal representation
// otherwise.
func processRUSER(p *process.Process) (string, error) {
	return process.LookupUID(p.Status.Uids[0])
}

// processName returns the name of process p in the format "[$name]".
func processName(p *process.Process) (string, error) {
	return fmt.Sprintf("[%s]", p.Status.Name), nil
}

// processARGS returns the command of p with all its arguments.
func processARGS(p *process.Process) (string, error) {
	// ps (1) returns "[$name]" if command/args are empty
	if p.CmdLine[0] == "" {
		return processName(p)
	}
	return strings.Join(p.CmdLine, " "), nil
}

// processCOMM returns the command name (i.e., executable name) of process p.
func processCOMM(p *process.Process) (string, error) {
	// ps (1) returns "[$name]" if command/args are empty
	if p.CmdLine[0] == "" {
		return processName(p)
	}
	spl := strings.Split(p.CmdLine[0], "/")
	return spl[len(spl)-1], nil
}

// processNICE returns the nice value of process p.
func processNICE(p *process.Process) (string, error) {
	return p.Stat.Nice, nil
}

// processPID returns the process ID of process p.
func processPID(p *process.Process) (string, error) {
	return p.Pid, nil
}

// processPGID returns the process group ID of process p.
func processPGID(p *process.Process) (string, error) {
	return p.Stat.Pgrp, nil
}

// processPCPU returns how many percent of the CPU time process p uses as
// a three digit float as string.
func processPCPU(p *process.Process) (string, error) {
	elapsed, err := p.ElapsedTime()
	if err != nil {
		return "", err
	}
	cpu, err := p.CPUTime()
	if err != nil {
		return "", err
	}
	pcpu := 100 * cpu.Seconds() / elapsed.Seconds()

	return strconv.FormatFloat(pcpu, 'f', 3, 64), nil
}

// processETIME returns the elapsed time since the process was started.
func processETIME(p *process.Process) (string, error) {
	elapsed, err := p.ElapsedTime()
	if err != nil {
		return "", nil
	}
	return fmt.Sprintf("%v", elapsed), nil
}

// processTIME returns the cumulative CPU time of process p.
func processTIME(p *process.Process) (string, error) {
	cpu, err := p.CPUTime()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", cpu), nil
}

// processTTY returns the controlling tty (terminal) of process p.
func processTTY(p *process.Process) (string, error) {
	ttyNr, err := strconv.ParseUint(p.Stat.TtyNr, 10, 64)
	if err != nil {
		return "", nil
	}

	tty, err := dev.FindTTY(ttyNr)
	if err != nil {
		return "", nil
	}

	ttyS := "?"
	if tty != nil {
		ttyS = strings.TrimPrefix(tty.Path, "/dev/")
	}
	return ttyS, nil
}

// processVSZ returns the virtual memory size of process p in KiB (1024-byte
// units).
func processVSZ(p *process.Process) (string, error) {
	vmsize, err := strconv.Atoi(p.Stat.Vsize)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", vmsize/1024), nil
}

// parseCAP parses cap (a string bit mask) and returns the associated set of
// capabilities.  If all capabilties are set, "full" is returned.  If no
// capability is enabled, "none" is returned.
func parseCAP(cap string) (string, error) {
	mask, err := strconv.ParseUint(cap, 16, 64)
	if err != nil {
		return "", err
	}
	if mask == capabilities.FullCAPs {
		return "full", nil
	}
	caps := capabilities.TranslateMask(mask)
	if len(caps) == 0 {
		return "none", nil
	}
	sort.Strings(caps)
	return strings.Join(caps, ","), nil
}

// processCAPAMB returns the set of ambient capabilties associated with
// process p.  If all capabilties are set, "full" is returned.  If no
// capability is enabled, "none" is returned.
func processCAPAMB(p *process.Process) (string, error) {
	return parseCAP(p.Status.CapAmb)
}

// processCAPINH returns the set of inheritable capabilties associated with
// process p.  If all capabilties are set, "full" is returned.  If no
// capability is enabled, "none" is returned.
func processCAPINH(p *process.Process) (string, error) {
	return parseCAP(p.Status.CapInh)
}

// processCAPPRM returns the set of permitted capabilties associated with
// process p.  If all capabilties are set, "full" is returned.  If no
// capability is enabled, "none" is returned.
func processCAPPRM(p *process.Process) (string, error) {
	return parseCAP(p.Status.CapPrm)
}

// processCAPEFF returns the set of effective capabilties associated with
// process p.  If all capabilties are set, "full" is returned.  If no
// capability is enabled, "none" is returned.
func processCAPEFF(p *process.Process) (string, error) {
	return parseCAP(p.Status.CapEff)
}

// processCAPBND returns the set of bounding capabilties associated with
// process p.  If all capabilties are set, "full" is returned.  If no
// capability is enabled, "none" is returned.
func processCAPBND(p *process.Process) (string, error) {
	return parseCAP(p.Status.CapBnd)
}

// processSECCOMP returns the seccomp mode of the process (i.e., disabled,
// strict or filter) or "?" if /proc/$pid/status.seccomp has a unknown value.
func processSECCOMP(p *process.Process) (string, error) {
	switch p.Status.Seccomp {
	case "0":
		return "disabled", nil
	case "1":
		return "strict", nil
	case "2":
		return "filter", nil
	default:
		return "?", nil
	}
}

// processLABEL returns the process label of process p or "?" if the system
// doesn't support labeling.
func processLABEL(p *process.Process) (string, error) {
	return p.Label, nil
}

// processHPID returns the PID of the corresponding host process of the
// (container) or "?" if no corresponding process could be found.
func processHPID(p *process.Process) (string, error) {
	if hp := findHostProcess(p); hp != nil {
		return hp.Pid, nil
	}
	return "?", nil
}

// processHUSER returns the effective user ID of the corresponding host process
// of the (container) or "?" if no corresponding process could be found.
func processHUSER(p *process.Process) (string, error) {
	if hp := findHostProcess(p); hp != nil {
		return hp.Huser, nil
	}
	return "?", nil
}

// processHGROUP returns the effective group ID of the corresponding host
// process of the (container) or "?" if no corresponding process could be
// found.
func processHGROUP(p *process.Process) (string, error) {
	if hp := findHostProcess(p); hp != nil {
		return hp.Hgroup, nil
	}
	return "?", nil
}

// processState returns the process state of process p.
func processState(p *process.Process) (string, error) {
	return p.Status.State, nil
}
