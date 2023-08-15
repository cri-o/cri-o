//go:build test
// +build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package oci

import (
	"github.com/cri-o/cri-o/pkg/config"
)

// SetState sets the container state
func (c *Container) SetState(state *ContainerState) {
	c.state = state
}

// SetStateAndSpoofPid sets the container state
// as well as configures the ProcessInformation to succeed
// useful for tests that don't care about pid handling
func (c *Container) SetStateAndSpoofPid(state *ContainerState) {
	// we do this hack because most of the tests
	// don't care to set a Pid.
	// but rely on calling Pid()
	if state.Pid == 0 {
		state.Pid = 1
		state.SetInitPid(state.Pid) // nolint:errcheck
	}
	c.state = state
}

type RuntimeOCI struct {
	*runtimeOCI
}

func NewRuntimeOCI(r *Runtime, handler *config.RuntimeHandler) RuntimeOCI {
	return RuntimeOCI{
		runtimeOCI: &runtimeOCI{
			Runtime: r,
			root:    handler.RuntimeRoot,
			handler: handler,
		},
	}
}
