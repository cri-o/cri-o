//go:build test

package oci

// SetRuntimeImplForContainer injects a RuntimeImpl for the provided container.
func (r *Runtime) SetRuntimeImplForContainer(c *Container, impl RuntimeImpl) {
	r.runtimeImplMapMutex.Lock()
	defer r.runtimeImplMapMutex.Unlock()

	r.runtimeImplMap[c.ID()] = impl
}
