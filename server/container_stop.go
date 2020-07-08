package server

import (
	"fmt"
	"strings"

	"github.com/cri-o/cri-o/internal/log"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// StopContainer stops a running container with a grace period (i.e., timeout).
func (s *Server) StopContainer(ctx context.Context, req *pb.StopContainerRequest) (*pb.StopContainerResponse, error) {
	log.Infof(ctx, "Stopping container: %s", req.GetContainerId())
	// save container description to print
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerId, err)
	}

	sandbox := s.getSandbox(c.Sandbox())
	if strings.Contains(sandbox.RuntimeHandler(), runtimeHandlerHighPerformance) && shouldCPULoadBalancingBeDisabled(sandbox.Annotations()) {
		if err := setCPUSLoadBalancing(c, true, schedDomainDir); err != nil {
			return nil, fmt.Errorf("failed to set back the container %q CPUs load balancing to true: %v", c.ID(), err)
		}
	}

	_, err = s.ContainerServer.ContainerStop(ctx, req.ContainerId, req.Timeout)
	if err != nil {
		return nil, err
	}

	log.Infof(ctx, "Stopped container %s: %s", c.ID(), c.Description())
	return &pb.StopContainerResponse{}, nil
}
