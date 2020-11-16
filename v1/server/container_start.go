package server

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/v1/oci"
	"github.com/cri-o/cri-o/v1/runtimehandlerhooks"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// StartContainer starts the container.
func (s *Server) StartContainer(ctx context.Context, req *pb.StartContainerRequest) (resp *pb.StartContainerResponse, retErr error) {
	log.Infof(ctx, "Starting container: %s", req.GetContainerId())
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerId, err)
	}
	state := c.State()
	if state.Status != oci.ContainerStateCreated {
		return nil, fmt.Errorf("container %s is not in created state: %s", c.ID(), state.Status)
	}

	sandbox := s.getSandbox(c.Sandbox())
	hooks := runtimehandlerhooks.GetRuntimeHandlerHooks(sandbox.RuntimeHandler())

	defer func() {
		// if the call to StartContainer fails below we still want to fill
		// some fields of a container status. In particular, we're going to
		// adjust container started/finished time and set an error to be
		// returned in the Reason field for container status call.
		if retErr != nil {
			c.SetStartFailed(retErr)
			if hooks != nil {
				if err := hooks.PreStop(ctx, c, sandbox); err != nil {
					log.Warnf(ctx, "failed to run pre-stop hook for container %q: %v", c.ID(), err)
				}
			}
		}
		if err := s.ContainerStateToDisk(c); err != nil {
			log.Warnf(ctx, "unable to write containers %s state to disk: %v", c.ID(), err)
		}
	}()

	if hooks != nil {
		if err := hooks.PreStart(ctx, c, sandbox); err != nil {
			return nil, fmt.Errorf("failed to run pre-start hook for container %q: %v", c.ID(), err)
		}
	}

	if err := s.Runtime().StartContainer(c); err != nil {
		return nil, fmt.Errorf("failed to start container %s: %v", c.ID(), err)
	}

	log.Infof(ctx, "Started container %s: %s", c.ID(), c.Description())
	return &pb.StartContainerResponse{}, nil
}
