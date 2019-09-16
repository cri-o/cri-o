package server

import (
	"github.com/cri-o/cri-o/internal/pkg/log"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// StopContainer stops a running container with a grace period (i.e., timeout).
func (s *Server) StopContainer(ctx context.Context, req *pb.StopContainerRequest) (resp *pb.StopContainerResponse, err error) {
	// save container description to print
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, err
	}
	description := c.Description()

	_, err = s.ContainerServer.ContainerStop(ctx, req.ContainerId, req.Timeout)
	if err != nil {
		return nil, err
	}

	log.Infof(ctx, "stopped container: %s", description)
	resp = &pb.StopContainerResponse{}
	return resp, nil
}
