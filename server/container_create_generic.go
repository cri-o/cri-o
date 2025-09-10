//go:build windows || darwin || freebsd

package server

import (
	"context"

	"go.podman.io/storage/pkg/idtools"
	"github.com/cri-o/cri-o/internal/oci"
)

// createContainerPlatform performs platform dependent intermediate steps before calling the container's oci.Runtime().CreateContainer()
func (s *Server) createContainerPlatform(ctx context.Context, container *oci.Container, cgroupParent string, idMappings *idtools.IDMappings) error {
	return s.ContainerServer.Runtime().CreateContainer(ctx, container, cgroupParent, false)
}
