package server

import (
	"fmt"

	"github.com/pkg/errors"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ReopenContainerLog reopens the containers log file
func (s *Server) ReopenContainerLog(ctx context.Context, req *types.ReopenContainerLogRequest) error {
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return fmt.Errorf("could not find container %s: %w", req.ContainerId, err)
	}

	if err := c.IsAlive(); err != nil {
		return errors.Errorf("container is not created or running: %v", err)
	}

	if err := s.ContainerServer.Runtime().ReopenContainerLog(ctx, c); err != nil {
		return err
	}
	return nil
}
