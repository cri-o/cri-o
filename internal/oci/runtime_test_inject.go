//go:build test

package oci

import (
	"context"

	conmonClient "github.com/containers/conmon-rs/pkg/client"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

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

// ConmonServerStopper stops the conmon-rs server of a pod.
type ConmonServerStopper = conmonServerStopper

// ShutdownConmonServer stops the conmon-rs server of a pod.
func ShutdownConmonServer(ctx context.Context, client ConmonServerStopper) error {
	return shutdownConmonServer(ctx, client)
}

// ConmonFeatureChecker reports the features of the conmon-rs server of a pod.
type ConmonFeatureChecker = conmonFeatureChecker

// RequireStreamingProtocols fails with conmonClient.ErrUnsupported if the
// conmon-rs server does not speak the streaming protocols of the kubelet.
func RequireStreamingProtocols(client ConmonFeatureChecker) error {
	return requireStreamingProtocols(client)
}

// ExecSyncResponse turns the result of a conmon-rs exec sync into the
// response of the kubelet.
func ExecSyncResponse(res *conmonClient.ExecContainerResult) (*types.ExecSyncResponse, error) {
	return execSyncResponse(res)
}

// UnsupportedError adds errors.ErrUnsupported to an error of the conmon-rs
// client which tells that the server does not support a feature.
func UnsupportedError(err error) error {
	return unsupportedError(err)
}

// HasRuntimeImpl returns whether a RuntimeImpl exists for the container ID.
func (r *Runtime) HasRuntimeImpl(containerID string) bool {
	r.runtimeImplMapMutex.RLock()
	defer r.runtimeImplMapMutex.RUnlock()

	_, ok := r.runtimeImplMap[containerID]

	return ok
}
