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

	if impl == nil {
		delete(r.runtimeImplMap, containerID)

		return
	}

	r.runtimeImplMap[containerID] = impl
}

// SetRuntimeImplForContainer injects a RuntimeImpl for the provided container.
func (r *Runtime) SetRuntimeImplForContainer(c *Container, impl RuntimeImpl) {
	r.SetRuntimeImpl(c.ID(), impl)
}
