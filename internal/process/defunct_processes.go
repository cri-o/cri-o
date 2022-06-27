package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// ProcessFS is the process file system.
const ProcessFS = "/proc"

// Stat represents status information of a process from /proc/[pid]/stat.
type Stat struct {
	// Comm is the command name (usually the executable filename).
	Comm string

	// State is the state of the process.
	State string
}

// DefunctProcesses returns the number of zombie processes in the node.
func DefunctProcesses() (defunctCount uint, retErr error) {
	return DefunctProcessesForPath(ProcessFS)
}

// DefunctProcessesForPath retrieves the number of zombie processes from
// a specific process filesystem.
func DefunctProcessesForPath(path string) (defunctCount uint, retErr error) {
	directories, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer directories.Close()

	names, err := directories.Readdirnames(-1)
	if err != nil {
		return 0, err
	}

	for _, name := range names {
		// Processes have numeric names. If the name cannot
		// be parsed to an int, it is not a process name.
		if _, err := strconv.ParseInt(name, 10, 0); err != nil {
			continue
		}

		stat, err := processStats(path, name)
		if err != nil {
			logrus.Debugf("Failed to get the status of process with PID %s: %v", name, err)
			continue
		}
		if stat.State == "Z" {
			logrus.Warnf("Found defunct process with PID %s (%s)", name, stat.Comm)
			defunctCount++
		}
	}
	return defunctCount, nil
}

// processStats returns status information of a process as defined in /proc/[pid]/stat
func processStats(fsPath, pid string) (*Stat, error) {
	bytes, err := os.ReadFile(filepath.Join(fsPath, pid, "stat"))
	if err != nil {
		return nil, err
	}
	data := string(bytes)

	// /proc/[PID]/stat format is described in proc(5). The second field is process name,
	// enclosed in parentheses, and it can contain parentheses inside. No other fields
	// can have parentheses, so look for the last ')'.
	i := strings.LastIndexByte(data, ')')
	if i <= 2 || i >= len(data)-1 {
		return nil, fmt.Errorf("invalid stat data (no comm): %q", data)
	}

	parts := strings.SplitN(data[:i], " (", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid stat data (no comm): %q", data)
	}

	return &Stat{
		// The command name is field 2.
		Comm: parts[1],

		// The state is field 3, which is the first two fields and a space after.
		State: string(data[i+2]),
	}, nil
}
