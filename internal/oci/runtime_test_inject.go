//go:build test

package oci

import (
	conmonClient "github.com/containers/conmon-rs/pkg/client"

	"github.com/cri-o/cri-o/pkg/config"
)

// NewTestRuntime creates a Runtime suitable for unit tests,
// without requiring a config or filesystem setup.
func NewTestRuntime() *Runtime {
	return &Runtime{
		runtimeImplMap: make(map[string]RuntimeImpl),
	}
}

// SetRuntimeImpl registers a RuntimeImpl for the given container ID.
func (r *Runtime) SetRuntimeImpl(containerID string, impl RuntimeImpl) {
	r.runtimeImplMapMutex.Lock()
	defer r.runtimeImplMapMutex.Unlock()

	r.runtimeImplMap[containerID] = impl
}

// MonitorEnv returns the environment of the container monitor of the handler.
func MonitorEnv(handler *config.RuntimeHandler) []string {
	return monitorEnv(handler)
}

// NewConmonServerConfig returns the conmon-rs server configuration for the
// infra container c.
func NewConmonServerConfig(
	r *Runtime,
	handler *config.RuntimeHandler,
	c *Container,
) *conmonClient.ConmonServerConfig {
	return newConmonServerConfig(r, handler, c, handler.RuntimeRoot)
}

// StopAfterStdinEOF returns whether conmon-rs closes the container stdin
// once an attach client closed its stdin.
func StopAfterStdinEOF(c *Container) bool {
	return stopAfterStdinEOF(c)
}
