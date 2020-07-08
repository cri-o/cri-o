package server

import (
	"fmt"
	"strings"

	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
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
	useHighPerformanceRuntimeHandler := strings.Contains(
		sandbox.RuntimeHandler(),
		runtimeHandlerHighPerformance,
	)
	defer func() {
		// if the call to StartContainer fails below we still want to fill
		// some fields of a container status. In particular, we're going to
		// adjust container started/finished time and set an error to be
		// returned in the Reason field for container status call.
		if retErr != nil {
			c.SetStartFailed(retErr)
		}
		if err := s.ContainerStateToDisk(c); err != nil {
			log.Warnf(ctx, "unable to write containers %s state to disk: %v", c.ID(), err)
		}

		if useHighPerformanceRuntimeHandler && shouldCPULoadBalancingBeDisabled(sandbox.Annotations()) {
			if err := setCPUSLoadBalancing(c, true, schedDomainDir); err != nil {
				log.Warnf(ctx, "failed to set the container %q CPUs load balancing to true: %v", c.ID(), err)
			}
		}
	}()

	if useHighPerformanceRuntimeHandler && shouldCPULoadBalancingBeDisabled(sandbox.Annotations()) {
		if err := setCPUSLoadBalancing(c, false, schedDomainDir); err != nil {
			return nil, fmt.Errorf("failed to set the container %q CPUs load balancing to false: %v", c.ID(), err)
		}
	}

	if err := s.Runtime().StartContainer(c); err != nil {
		return nil, fmt.Errorf("failed to start container %s: %v", c.ID(), err)
	}

	log.Infof(ctx, "Started container %s: %s", c.ID(), c.Description())
	return &pb.StartContainerResponse{}, nil
}
