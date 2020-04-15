// +build linux

package libpod

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"github.com/containers/libpod/libpod/define"
	"github.com/containers/libpod/pkg/rootless"
	"github.com/containers/psgo"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Top gathers statistics about the running processes in a container. It returns a
// []string for output
func (c *Container) Top(descriptors []string) ([]string, error) {
	if c.config.NoCgroups {
		return nil, errors.Wrapf(define.ErrNoCgroups, "cannot run top on container %s as it did not create a cgroup", c.ID())
	}

	conStat, err := c.State()
	if err != nil {
		return nil, errors.Wrapf(err, "unable to look up state for %s", c.ID())
	}
	if conStat != define.ContainerStateRunning {
		return nil, errors.Errorf("top can only be used on running containers")
	}

	// Also support comma-separated input.
	psgoDescriptors := []string{}
	for _, d := range descriptors {
		for _, s := range strings.Split(d, ",") {
			if s != "" {
				psgoDescriptors = append(psgoDescriptors, s)
			}
		}
	}

	// If we encountered an ErrUnknownDescriptor error, fallback to executing
	// ps(1). This ensures backwards compatibility to users depending on ps(1)
	// and makes sure we're ~compatible with docker.
	output, psgoErr := c.GetContainerPidInformation(psgoDescriptors)
	if psgoErr == nil {
		return output, nil
	}
	if errors.Cause(psgoErr) != psgo.ErrUnknownDescriptor {
		return nil, psgoErr
	}

	output, err = c.execPS(descriptors)
	if err != nil {
		return nil, errors.Wrapf(err, "error executing ps(1) in the container")
	}

	// Trick: filter the ps command from the output instead of
	// checking/requiring PIDs in the output.
	filtered := []string{}
	cmd := strings.Join(descriptors, " ")
	for _, line := range output {
		if !strings.Contains(line, cmd) {
			filtered = append(filtered, line)
		}
	}

	return filtered, nil
}

// GetContainerPidInformation returns process-related data of all processes in
// the container.  The output data can be controlled via the `descriptors`
// argument which expects format descriptors and supports all AIXformat
// descriptors of ps (1) plus some additional ones to for instance inspect the
// set of effective capabilities.  Each element in the returned string slice
// is a tab-separated string.
//
// For more details, please refer to github.com/containers/psgo.
func (c *Container) GetContainerPidInformation(descriptors []string) ([]string, error) {
	pid := strconv.Itoa(c.state.PID)
	// TODO: psgo returns a [][]string to give users the ability to apply
	//       filters on the data.  We need to change the API here and the
	//       varlink API to return a [][]string if we want to make use of
	//       filtering.
	opts := psgo.JoinNamespaceOpts{FillMappings: rootless.IsRootless()}

	psgoOutput, err := psgo.JoinNamespaceAndProcessInfoWithOptions(pid, descriptors, &opts)
	if err != nil {
		return nil, err
	}
	res := []string{}
	for _, out := range psgoOutput {
		res = append(res, strings.Join(out, "\t"))
	}
	return res, nil
}

// execPS executes ps(1) with the specified args in the container.
func (c *Container) execPS(args []string) ([]string, error) {
	rPipe, wPipe, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer wPipe.Close()
	defer rPipe.Close()

	rErrPipe, wErrPipe, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer wErrPipe.Close()
	defer rErrPipe.Close()

	streams := new(define.AttachStreams)
	streams.OutputStream = wPipe
	streams.ErrorStream = wErrPipe
	streams.AttachOutput = true
	streams.AttachError = true

	stdout := []string{}
	go func() {
		scanner := bufio.NewScanner(rPipe)
		for scanner.Scan() {
			stdout = append(stdout, scanner.Text())
		}
	}()
	stderr := []string{}
	go func() {
		scanner := bufio.NewScanner(rErrPipe)
		for scanner.Scan() {
			stderr = append(stderr, scanner.Text())
		}
	}()

	cmd := append([]string{"ps"}, args...)
	config := new(ExecConfig)
	config.Command = cmd
	ec, err := c.Exec(config, streams, nil)
	if err != nil {
		return nil, err
	} else if ec != 0 {
		return nil, errors.Errorf("Runtime failed with exit status: %d and output: %s", ec, strings.Join(stderr, " "))
	}

	if logrus.GetLevel() >= logrus.DebugLevel {
		// If we're running in debug mode or higher, we might want to have a
		// look at stderr which includes debug logs from conmon.
		for _, log := range stderr {
			logrus.Debugf("%s", log)
		}
	}

	return stdout, nil
}
