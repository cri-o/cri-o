//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package oci

import (
	"context"
	"syscall"

	task "github.com/containerd/containerd/api/runtime/task/v2"

	"github.com/cri-o/cri-o/pkg/config"
)

// SetState sets the container state.
func (c *Container) SetState(state *ContainerState) {
	c.state = state
}

// SetStateAndSpoofPid sets the container state
// as well as configures the ProcessInformation to succeed
// useful for tests that don't care about pid handling.
func (c *Container) SetStateAndSpoofPid(state *ContainerState) {
	// we do this hack because most of the tests
	// don't care to set a Pid.
	// but rely on calling Pid()
	if state.Pid == 0 {
		state.Pid = 1
		state.SetInitPid(state.Pid) //nolint:errcheck // error not relevant in test setup
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

type RuntimeVM struct {
	*runtimeVM
}

func NewRuntimeVM() RuntimeVM {
	return RuntimeVM{
		runtimeVM: &runtimeVM{
			ctx:     context.Background(),
			handler: &config.RuntimeHandler{},
			ctrs:    make(map[string]containerInfo),
		},
	}
}

func (r RuntimeVM) UpdateContainerStatus(ctx context.Context, c *Container) error {
	return r.updateContainerStatus(ctx, c)
}

func (r RuntimeVM) StopContainer(ctx context.Context, c *Container, timeout int64) error {
	return r.runtimeVM.StopContainer(ctx, c, timeout)
}

func (r RuntimeVM) DeleteContainer(ctx context.Context, c *Container) error {
	return r.runtimeVM.DeleteContainer(ctx, c)
}

func (r RuntimeVM) HasTask() bool {
	return r.task != nil
}

func NewRuntimeVMWithTask(t task.TaskService) RuntimeVM {
	return RuntimeVM{
		runtimeVM: &runtimeVM{
			task: t,
			ctx:  context.Background(),
			ctrs: make(map[string]containerInfo),
		},
	}
}

func (r RuntimeVM) Kill(ctrID, execID string, signal syscall.Signal) error {
	return r.kill(ctrID, execID, signal)
}
