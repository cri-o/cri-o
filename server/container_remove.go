package server

import (
	"github.com/cri-o/cri-o/internal/pkg/log"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (s *Server) RemoveContainer(ctx context.Context, req *pb.RemoveContainerRequest) (resp *pb.RemoveContainerResponse, err error) {
	// save container description to print
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, err
	}

	_, err = s.ContainerServer.Remove(ctx, req.ContainerId, true)
	if err != nil {
		return nil, err
	}

	log.Infof(ctx, "Removed container %s", c.Description())
	resp = &pb.RemoveContainerResponse{}
	return resp, nil
}
