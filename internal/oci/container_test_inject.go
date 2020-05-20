// +build test
// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package oci

// SetState sets the container state
func (c *Container) SetState(state *ContainerState) {
	c.stateLock.Lock()
	c.state = state
	c.stateLock.Unlock()
}
