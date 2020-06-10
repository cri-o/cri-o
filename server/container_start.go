package server

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// StartContainer starts the container.
func (s *Server) StartContainer(ctx context.Context, req *pb.StartContainerRequest) (resp *pb.StartContainerResponse, err error) {
	log.Infof(ctx, "Starting container: %s", req.GetContainerId())
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerId, err)
	}

	defer func() {
		// we should only do this cleanup if the start actually failed,
		// rather than failing because the user tried to start a container that wasn't in the created state
		if errors.Is(err, oci.ErrContainerNotCreated) {
			return
		}
		// if the call to StartContainer fails below we still want to fill
		// some fields of a container status. In particular, we're going to
		// adjust container started/finished time and set an error to be
		// returned in the Reason field for container status call.
		if err != nil {
			c.SetStartFailed(err)
		}
		if err := s.ContainerStateToDisk(c); err != nil {
			log.Warnf(ctx, "unable to write containers %s state to disk: %v", c.ID(), err)
		}
	}()

	err = s.Runtime().StartContainer(c)
	if err != nil {
		return nil, fmt.Errorf("failed to start container %s: %v", c.ID(), err)
	}

	log.Infof(ctx, "Started container %s: %s", c.ID(), c.Description())
	resp = &pb.StartContainerResponse{}
	return resp, nil
}
