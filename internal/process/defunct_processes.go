package process

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ProcessFS is the process file system.
const ProcessFS = "/proc"

// Stat represents status information of a process from /proc/[pid]/stat.
type Stat struct {
	// Pid is the PID of the process
	Pid int

	// Comm is the command name (usually the executable filename).
	Comm string

	// State is the state of the process.
	State string

	// PPid is the parent PID of the process
	PPid int
}

// ParseDefunctProcesses returns the number of defunct processes on the node,
// as well as the number of defunct children of the current running process.
func ParseDefunctProcesses() (defunctCount uint, defunctChildren []int, retErr error) {
	return ParseDefunctProcessesForPathAndParent(ProcessFS, os.Getpid())
}

// ParseDefunctProcessesForPath retrieves the number of zombie processes from
// a specific process filesystem, as well as the number of defunct children of a given parent.
func ParseDefunctProcessesForPathAndParent(path string, parent int) (defunctCount uint, defunctChildren []int, retErr error) {
	directories, err := os.Open(path)
	if err != nil {
		return 0, defunctChildren, err
	}
	defer directories.Close()

	names, err := directories.Readdirnames(-1)
	if err != nil {
		return 0, defunctChildren, err
	}

	for _, name := range names {
		// Processes have numeric names. If the name cannot
		// be parsed to an int, it is not a process name.
		pid, err := strconv.ParseInt(name, 10, 0)
		if err != nil {
			continue
		}

		stat, err := processStats(path, name)
		if err != nil {
			continue
		}
		if stat.State == "Z" {
			defunctCount++
			logrus.Warnf("Found defunct process with PID %s (%s)", name, stat.Comm)
			if stat.PPid == parent {
				defunctChildren = append(defunctChildren, int(pid))
			}
		}
	}
	return defunctCount, defunctChildren, nil
}

// processStats returns status information of a process as defined in /proc/[pid]/stat
func processStats(fsPath, pid string) (*Stat, error) {
	bytes, err := ioutil.ReadFile(filepath.Join(fsPath, pid, "stat"))
	if err != nil {
		return nil, err
	}
	data := string(bytes)

	// /proc/[PID]/stat format is described in proc(5). The second field is process name,
	// enclosed in parentheses, and it can contain parentheses inside. No other fields
	// can have parentheses, so look for the last ')'.
	commEnd := strings.LastIndexByte(data, ')')
	if commEnd <= 2 || commEnd >= len(data)-1 {
		return nil, errors.Errorf("invalid stat data (no comm): %q", data)
	}

	parts := strings.SplitN(data[:commEnd], " (", 2)
	if len(parts) != 2 {
		return nil, errors.Errorf("invalid stat data (no comm): %q", data)
	}

	stateIdx := commEnd + 2

	// the fourth field is PPid, and we can start looking after the space after State
	ppidBegin := stateIdx + 2
	ppidEnd := strings.IndexByte(data[ppidBegin:], ' ')

	ppid, err := strconv.ParseInt(data[ppidBegin:ppidBegin+ppidEnd], 10, 0)
	if err != nil {
		return nil, errors.Errorf("invalid stat data (invalid ppid): %q", data[stateIdx+2:ppidEnd])
	}

	return &Stat{
		// The command name is field 2.
		Comm: parts[1],

		// The state is field 3, which is the first two fields and a space after.
		State: string(data[stateIdx]),

		PPid: int(ppid),
	}, nil
}
