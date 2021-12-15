package server

import (
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (s *Server) RemoveContainer(ctx context.Context, req *types.RemoveContainerRequest) error {
	log.Infof(ctx, "Removing container: %s", req.ContainerId)
	// save container description to print
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerId, err)
	}

	cStatus := c.State()
	switch cStatus.Status {
	case oci.ContainerStatePaused:
		return errors.Errorf("cannot remove paused container %s", c.ID())
	case oci.ContainerStateCreated, oci.ContainerStateRunning:
		if err := s.ContainerServer.StopContainer(ctx, c, 10); err != nil {
			return errors.Wrapf(err, "unable to stop container %s", c.ID())
		}
	}

	if err := s.ContainerServer.RemoveAndDeleteContainer(ctx, c); err != nil {
		return err
	}

	log.Infof(ctx, "Removed container %s: %s", c.ID(), c.Description())
	return nil
}
