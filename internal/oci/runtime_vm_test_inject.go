//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package oci

import (
	"context"
	"syscall"

	task "github.com/containerd/containerd/api/runtime/task/v2"
)

// RuntimeVM wraps runtimeVM for testing.
type RuntimeVM struct {
	*runtimeVM
}

// NewRuntimeVMWithTask creates a runtimeVM with an injected task service,
// used to test behavior when the shim connection is in specific states.
func NewRuntimeVMWithTask(t task.TaskService) RuntimeVM {
	return RuntimeVM{
		runtimeVM: &runtimeVM{
			task: t,
			ctx:  context.Background(),
			ctrs: make(map[string]containerInfo),
		},
	}
}

// Kill exposes the unexported kill() for testing.
func (r RuntimeVM) Kill(ctrID, execID string, signal syscall.Signal) error {
	return r.kill(ctrID, execID, signal)
}
