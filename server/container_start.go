package server

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// StartContainer starts the container.
func (s *Server) StartContainer(ctx context.Context, req *pb.StartContainerRequest) (resp *pb.StartContainerResponse, err error) {
	cs := lib.ContainerServer()
	c, err := cs.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerId, err)
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
		if err := cs.ContainerStateToDisk(c); err != nil {
			log.Warnf(ctx, "unable to write containers %s state to disk: %v", c.ID(), err)
		}
	}()

	// in the event a container is created, conmon is oom killed, then the container
	// is started, we want to start the watcher on it, and appropriately kill it.
	// this situation is VERY unlikely, but conmonmon exists because the kernel OOM killer
	// can't be trusted
	if err := cs.MonitorConmon(c); err != nil {
		log.Debugf(ctx, "failed to add conmon for %s to monitor: %v", c.ID(), err)
	}

	err = cs.Runtime().StartContainer(c)
	if err != nil {
		return nil, fmt.Errorf("failed to start container %s: %v", c.ID(), err)
	}

	log.Infof(ctx, "Started container %s: %s", c.ID(), c.Description())
	resp = &pb.StartContainerResponse{}
	return resp, nil
}
