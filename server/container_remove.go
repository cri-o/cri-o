package server

import (
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/server/cri/types"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (s *Server) RemoveContainer(ctx context.Context, req *types.RemoveContainerRequest) error {
	log.Infof(ctx, "Removing container: %s", req.ContainerID)
	// save container description to print
	c, err := s.GetContainerFromShortID(req.ContainerID)
	if err != nil {
		return status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerID, err)
	}

	if _, err := s.ContainerServer.Remove(ctx, req.ContainerID, true); err != nil {
		return err
	}

	log.Infof(ctx, "Removed container %s: %s", c.ID(), c.Description())
	return nil
}
