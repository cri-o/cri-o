package server

import (
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/log"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (s *Server) RemoveContainer(ctx context.Context, req *pb.RemoveContainerRequest) (resp *pb.RemoveContainerResponse, err error) {
	cs := lib.ContainerServer()
	// save container description to print
	c, err := cs.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerId, err)
	}

	_, err = cs.Remove(ctx, req.ContainerId, s.config.ContainerExitsDir, true)
	if err != nil {
		return nil, err
	}

	cs.StopMonitoringConmon(c)

	log.Infof(ctx, "Removed container %s: %s", c.ID(), c.Description())
	resp = &pb.RemoveContainerResponse{}
	return resp, nil
}
