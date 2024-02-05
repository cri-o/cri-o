package oci

import (
	"bytes"
	"errors"
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

// getPidData reads the kernel's /proc entry for various data.
// inspiration for this function came from https://github.com/containers/psgo/blob/master/internal/proc/stat.go
// some credit goes to the psgo authors
func getPidData(pid int) (*StatData, error) {
	return getPidStatData(fmt.Sprintf("/proc/%d/stat", pid))
}

// GetPidStartTimeFromFile reads a file as if it were a /proc/$pid/stat file, looking for stime for PID.
// It is abstracted out to allow for unit testing
func GetPidStartTimeFromFile(file string) (string, error) {
	data, err := getPidStatData(file)
	if err != nil {
		return "", err
	}
	return data.StartTime, nil
}

func getPidStatData(file string) (*StatData, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", err, ErrNotFound)
	}
	// The command (2nd field) can have spaces, but is wrapped in ()
	// first, trim it
	commEnd := bytes.LastIndexByte(data, ')')
	if commEnd == -1 {
		return nil, fmt.Errorf("unable to find ')' in stat file: %w", ErrNotFound)
	}

	// skip space after the command
	iter := commEnd + 1
	// skip the character after the space
	iter++

	fields := bytes.Fields(data[iter:])
	if len(fields) <= statStartTimeLocation {
		return nil, errors.New("unable to parse stat file")
	}

	return &StatData{
		StartTime: string(fields[statStartTimeLocation-3]),
		State:     string(fields[statStateLocation-3]),
	}, nil
}
