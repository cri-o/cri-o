package oci

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"go.podman.io/common/pkg/cgroups"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
)

const (
	procStatFile = "/proc/%d/stat"

	// Fields from the /proc/<PID>/stat file. see:
	//   https://man7.org/linux/man-pages/man5/proc.5.html
	//
	// Field no. 3, the process state, such as "R", "S", "D", etc.
	// Field no. 22, the process start time, using clock ticks since the system boot.
	//
	// The index values are shifted three fields to the left
	// with the process name field skipped over during parsing.
	stateFieldIndex     = 0
	startTimeFieldIndex = 19
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

// SetSeccompProfilePath sets the seccomp profile path.
func (c *Container) SetSeccompProfilePath(pp string) {
	c.seccompProfilePath = pp
}

// SeccompProfilePath returns the seccomp profile path.
func (c *Container) SeccompProfilePath() string {
	return c.seccompProfilePath
}

// GetPidStartTimeFromFile reads a file as if it were a /proc/<PID>/stat file,
// looking for a process start time for a given PID. It is abstracted out to
// allow for unit testing.
func GetPidStartTimeFromFile(file string) (string, error) {
	_, startTime, err := getPidStatDataFromFile(file)

	return startTime, err
}

// getPidStartTime returns the process start time for a given PID.
func getPidStartTime(pid int) (string, error) {
	_, startTime, err := getPidStatDataFromFile(fmt.Sprintf(procStatFile, pid))

	return startTime, err
}

// getPidStatData returns the process state and start time for a given PID.
func getPidStatData(pid int) (string, string, error) { //nolint:gocritic // Ignore unnamedResult.
	return getPidStatDataFromFile(fmt.Sprintf(procStatFile, pid))
}

// getPidStatData parses the kernel's /proc/<PID>/stat file,
// looking for the process state and start time for a given PID.
func getPidStatDataFromFile(file string) (string, string, error) { //nolint:gocritic // Ignore unnamedResult.
	f, err := os.Open(file)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, 4096))
	if err != nil {
		return "", "", err
	}

	bracket := bytes.LastIndexByte(data, ')')
	if bracket < 0 {
		return "", "", fmt.Errorf("unable to find ')' in stat file: %w", ErrNotFound)
	}

	// Skip the process name and the white space after the right bracket.
	statFields := bytes.Fields(data[bracket+2:])

	if len(statFields) < startTimeFieldIndex+1 {
		return "", "", fmt.Errorf("unable to parse malformed stat file: %w", ErrNotFound)
	}

	return string(statFields[stateFieldIndex]), string(statFields[startTimeFieldIndex]), nil
}

// SetRuntimeUser sets the runtime user for the container.
func (c *Container) SetRuntimeUser(runtimeSpec *specs.Spec) {
	if runtimeSpec.Process == nil {
		logrus.Infof("Container %s is missing process attribute from the runtime specification", c.ID())

		return
	}

	user := runtimeSpec.Process.User
	supplementalGroups := make([]int64, 0, len(user.AdditionalGids))

	for _, gid := range user.AdditionalGids {
		supplementalGroups = append(supplementalGroups, int64(gid))
	}

	c.runtimeUser = &types.ContainerUser{
		Linux: &types.LinuxContainerUser{
			Uid:                int64(user.UID),
			Gid:                int64(user.GID),
			SupplementalGroups: supplementalGroups,
		},
	}
}
