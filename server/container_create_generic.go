// +build windows darwin

package server

import "github.com/cri-o/cri-o/internal/oci"

// createContainerPlatform performs platform dependent intermediate steps before calling the container's oci.Runtime().CreateContainer()
func (s *Server) createContainerPlatform(container *oci.Container, infraContainer *oci.Container, cgroupParent string) error {
	return s.Runtime().CreateContainer(container, cgroupParent)
}
