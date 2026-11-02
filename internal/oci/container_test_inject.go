//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package oci

import (
	"context"
	"syscall"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/ttrpc"

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

// NewRuntimeVMWithHandler creates a RuntimeVM for the given runtime handler.
func NewRuntimeVMWithHandler(handler *config.RuntimeHandler) RuntimeVM {
	return RuntimeVM{
		runtimeVM: &runtimeVM{
			ctx:     context.Background(),
			handler: handler,
			ctrs:    make(map[string]containerInfo),
		},
	}
}

func (r RuntimeVM) Kill(ctrID, execID string, signal syscall.Signal) error {
	return r.kill(ctrID, execID, signal)
}

// ShimDeleteArgs returns the arguments of the shim binary's delete command.
func ShimDeleteArgs(id, bundlePath string, debug bool) []string {
	return shimDeleteArgs(id, bundlePath, debug)
}

// CleanupAfterDeadShim runs the shim's delete command synchronously.
func (r RuntimeVM) CleanupAfterDeadShim(ctx context.Context, c *Container) error {
	return r.cleanupAfterDeadShim(ctx, c)
}

// SetTask sets the task service of the runtime.
func (r RuntimeVM) SetTask(t task.TaskService) {
	r.task = t
}

// SetClient sets the ttrpc client of the runtime.
func (r RuntimeVM) SetClient(cl *ttrpc.Client) {
	r.client = cl
}

// AddContainer registers a container in the runtime without any container IO.
func (r RuntimeVM) AddContainer(ctrID string) {
	r.Lock()
	r.ctrs[ctrID] = containerInfo{}
	r.Unlock()
}

// IsDeleting reports whether the runtime is deleting the container.
func (r RuntimeVM) IsDeleting(ctrID string) bool {
	return r.isDeleting(ctrID)
}
