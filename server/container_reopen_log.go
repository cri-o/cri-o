package server

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ReopenContainerLog reopens the containers log file
func (s *Server) ReopenContainerLog(ctx context.Context, req *types.ReopenContainerLogRequest) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	c, err := s.GetContainerFromShortID(ctx, req.ContainerId)
	if err != nil {
		return fmt.Errorf("could not find container %s: %w", req.ContainerId, err)
	}

	if err := s.ContainerServer.Runtime().UpdateContainerStatus(ctx, c); err != nil {
		return err
	}

	cState := c.State()
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return fmt.Errorf("container is not created or running")
	}

	if err := s.ContainerServer.Runtime().ReopenContainerLog(ctx, c); err != nil {
		return err
	}
	return nil
}
