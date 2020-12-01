// +build windows darwin

package server

import (
	"github.com/containers/storage/pkg/idtools"
	"github.com/cri-o/cri-o/internal/oci"
)

// createContainerPlatform performs platform dependent intermediate steps before calling the container's oci.Runtime().CreateContainer()
func (s *Server) createContainerPlatform(container *oci.Container, cgroupParent string, idMappings *idtools.IDMappings) error {
	return s.Runtime().CreateContainer(container, cgroupParent)
}
