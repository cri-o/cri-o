//go:build test

package oci

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
