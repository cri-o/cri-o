package server

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/pkg/log"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// StartContainer starts the container.
func (s *Server) StartContainer(ctx context.Context, req *pb.StartContainerRequest) (resp *pb.StartContainerResponse, err error) {
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, err
	}
	state := c.State()
	if state.Status != oci.ContainerStateCreated {
		return nil, fmt.Errorf("container %s is not in created state: %s", c.ID(), state.Status)
	}

	defer func() {
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

	log.Infof(ctx, "Started container: %s", c.Description())
	resp = &pb.StartContainerResponse{}
	return resp, nil
}
