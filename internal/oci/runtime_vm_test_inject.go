//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package oci

import (
	"context"
	"syscall"

	"github.com/cri-o/cri-o/pkg/config"
)

// RuntimeVM wraps the unexported runtimeVM for use in unit tests.
type RuntimeVM struct {
	*runtimeVM
}

// NewRuntimeVM creates a RuntimeVM with a nil task, simulating a post-restart
// state where the shim connection has not yet been re-established.
func NewRuntimeVM(handler *config.RuntimeHandler) RuntimeVM {
	return RuntimeVM{
		runtimeVM: &runtimeVM{
			ctx:     context.Background(),
			handler: handler,
			ctrs:    make(map[string]containerInfo),
		},
	}
}

// HasTask reports whether the underlying task connection is non-nil.
func (r RuntimeVM) HasTask() bool {
	return r.task != nil
}

// Kill calls the unexported kill() method so tests can verify the nil-task guard.
func (r RuntimeVM) Kill(ctrID, execID string, signal syscall.Signal) error {
	return r.kill(ctrID, execID, signal)
}

// ConnectTask calls the unexported connectTask() method so tests can verify
// the reconnect logic without a real shim.
func (r RuntimeVM) ConnectTask(ctx context.Context, c *Container) error {
	return r.connectTask(ctx, c)
}
