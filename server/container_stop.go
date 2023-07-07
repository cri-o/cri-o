package server

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/runtimehandlerhooks"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// StopContainer stops a running container with a grace period (i.e., timeout).
func (s *Server) StopContainer(ctx context.Context, req *types.StopContainerRequest) error {
	log.Infof(ctx, "Stopping container: %s (timeout: %ds)", req.ContainerId, req.Timeout)
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerId, err)
	}

	sandbox := s.getSandbox(c.Sandbox())
	hooks, err := runtimehandlerhooks.GetRuntimeHandlerHooks(ctx, &s.config, sandbox.RuntimeHandler(), sandbox.Annotations())
	if err != nil {
		return fmt.Errorf("failed to get runtime handler %q hooks", sandbox.RuntimeHandler())
	}

	if hooks != nil {
		if err := hooks.PreStop(ctx, c, sandbox); err != nil {
			return fmt.Errorf("failed to run pre-stop hook for container %q: %w", c.ID(), err)
		}
	}

	if err := s.stopContainer(ctx, c, req.Timeout); err != nil {
		return err
	}

	log.Infof(ctx, "Stopped container %s: %s", c.ID(), c.Description())
	return nil
}

// stopContainer stops a running container with a grace period (i.e., timeout).
func (s *Server) stopContainer(ctx context.Context, ctr *oci.Container, timeout int64) error {
	if ctr.StateNoLock().Status == oci.ContainerStatePaused {
		if err := s.Runtime().UnpauseContainer(ctx, ctr); err != nil {
			return fmt.Errorf("failed to stop container %s: %v", ctr.Name(), err)
		}
		if err := s.Runtime().UpdateContainerStatus(ctx, ctr); err != nil {
			return fmt.Errorf("failed to update container status %s: %v", ctr.Name(), err)
		}
	}

	if err := s.Runtime().StopContainer(ctx, ctr, timeout); err != nil {
		return fmt.Errorf("failed to stop container %s: %w", ctr.ID(), err)
	}
	if err := s.Runtime().UpdateContainerStatus(ctx, ctr); err != nil {
		return fmt.Errorf("failed to update container status %s: %w", ctr.ID(), err)
	}
	if err := s.StorageRuntimeServer().StopContainer(ctr.ID()); err != nil {
		return fmt.Errorf("failed to unmount container %s: %w", ctr.ID(), err)
	}

	if err := s.ContainerStateToDisk(ctx, ctr); err != nil {
		log.Warnf(ctx, "Unable to write containers %s state to disk: %v", ctr.ID(), err)
	}

	return nil
}
