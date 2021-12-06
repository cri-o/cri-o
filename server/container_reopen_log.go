package server

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ReopenContainerLog reopens the containers log file
func (s *Server) ReopenContainerLog(ctx context.Context, req *types.ReopenContainerLogRequest) error {
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return errors.Wrapf(err, "could not find container %s", req.ContainerId)
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
