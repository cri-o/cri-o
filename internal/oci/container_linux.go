package oci

import (
	"bytes"
	"fmt"
	"os"

	"github.com/containers/common/pkg/cgroups"
	"github.com/cri-o/cri-o/internal/log"
	"golang.org/x/net/context"
)

// CleanupConmonCgroup cleans up conmon's group when using cgroupfs.
func (c *Container) CleanupConmonCgroup(ctx context.Context) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	if c.spoofed {
		return
	}
	path := c.ConmonCgroupfsPath()
	if path == "" {
		return
	}
	cg, err := cgroups.Load(path)
	if err != nil {
		log.Infof(ctx, "Error loading conmon cgroup of container %s: %v", c.ID(), err)
		return
	}
	if err := cg.Delete(); err != nil {
		log.Infof(ctx, "Error deleting conmon cgroup of container %s: %v", c.ID(), err)
	}
}

// SetSeccompProfilePath sets the seccomp profile path
func (c *Container) SetSeccompProfilePath(pp string) {
	c.seccompProfilePath = pp
}

// SeccompProfilePath returns the seccomp profile path
func (c *Container) SeccompProfilePath() string {
	return c.seccompProfilePath
}

// getPidStartTime reads the kernel's /proc entry for stime for PID.
// inspiration for this function came from https://github.com/containers/psgo/blob/master/internal/proc/stat.go
// some credit goes to the psgo authors
func getPidStartTime(pid int) (string, error) {
	return GetPidStartTimeFromFile(fmt.Sprintf("/proc/%d/stat", pid))
}

// GetPidStartTime reads a file as if it were a /proc/$pid/stat file, looking for stime for PID.
// It is abstracted out to allow for unit testing
func GetPidStartTimeFromFile(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("%v: %w", err, ErrNotFound)
	}
	// The command (2nd field) can have spaces, but is wrapped in ()
	// first, trim it
	commEnd := bytes.LastIndexByte(data, ')')
	if commEnd == -1 {
		return "", fmt.Errorf("unable to find ')' in stat file: %w", ErrNotFound)
	}

	// start on the space after the command
	iter := commEnd + 1
	// for the number of fields between command and stime, trim the beginning word
	for field := 0; field < statStartTimeLocation-statCommField; field++ {
		// trim from the beginning to the character after the last space
		data = data[iter+1:]
		// find the next space
		iter = bytes.IndexByte(data, ' ')
		if iter == -1 {
			return "", fmt.Errorf("invalid number of entries found in stat file %s: %d: %w", file, field-1, ErrNotFound)
		}
	}

	// and return the startTime (not including the following space)
	return string(data[:iter]), nil
}
