package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/containers/storage/pkg/truncindex"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/runtimehandlerhooks"
)

// StopContainer stops a running container with a grace period (i.e., timeout).
func (s *Server) StopContainer(ctx context.Context, req *types.StopContainerRequest) (*types.StopContainerResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	log.Infof(ctx, "Stopping container: %s (timeout: %ds)", req.GetContainerId(), req.GetTimeout())

	c, err := s.GetContainerFromShortID(ctx, req.GetContainerId())
	if err != nil {
		// The StopContainer RPC is idempotent, and must not return an error if
		// the container has already been stopped. Ref:
		// https://github.com/kubernetes/cri-api/blob/c20fa40/pkg/apis/runtime/v1/api.proto#L67-L68
		if errors.Is(err, truncindex.ErrNotExist) {
			return &types.StopContainerResponse{}, nil
		}

		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.GetContainerId(), err)
	}

	if err := s.stopContainer(ctx, c, req.GetTimeout()); err != nil {
		return nil, err
	}

	log.Infof(ctx, "Stopped container %s: %s", c.ID(), c.Description())

	return &types.StopContainerResponse{}, nil
}

// stopContainer stops a running container with a grace period (i.e., timeout).
func (s *Server) stopContainer(ctx context.Context, ctr *oci.Container, timeout int64) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	sb := s.getSandbox(ctx, ctr.Sandbox())

	hooks := s.hooksRetriever.Get(ctx, sb.RuntimeHandler(), sb.Annotations())
	if hooks != nil {
		if err := hooks.PreStop(ctx, ctr, sb); err != nil {
			return fmt.Errorf("failed to run pre-stop hook for container %q: %w", ctr.ID(), err)
		}
	}

	if err := s.ContainerServer.Runtime().StopContainer(ctx, ctr, timeout); err != nil {
		return fmt.Errorf("failed to stop container %s: %w", ctr.ID(), err)
	}
	// Don't do post stop cleanup here. Instead, allow the inotify code in server/server.go to catch the exit and run post stop.
	// This has a couple of advantages:
	// - If the container is removed, but the sandbox is not considered stopped, then we stop each container first, even if it's already stopped, thus
	//   redoing this cleanup
	// - Conceptually, it's straightforward to expect these post stop actions to happen in just one place, and the inotify loop can be that single source.

	return nil
}

func (s *Server) postStopCleanup(ctx context.Context, ctr *oci.Container, sb *sandbox.Sandbox, hooks runtimehandlerhooks.RuntimeHandlerHooks) {
	if err := s.ContainerServer.StorageRuntimeServer().StopContainer(ctx, ctr.ID()); err != nil {
		log.Errorf(ctx, "Failed to unmount container %s: %v", ctr.ID(), err)
	}

	if hooks != nil {
		if err := hooks.PostStop(ctx, ctr, sb); err != nil {
			// The hook failure MUST NOT prevent the Pod termination
			log.Errorf(ctx, "Failed to run post-stop hook for container %s: %v", ctr.ID(), err)
		}
	}

	if err := s.nri.stopContainer(ctx, sb, ctr); err != nil {
		log.Warnf(ctx, "NRI stop container request of %s failed: %v", ctr.ID(), err)
	}
	// persist container state at the end, so there's no window where CRI-O reports the container
	// as stopped, but hasn't run post stop hooks.
	if err := s.ContainerStateToDisk(ctx, ctr); err != nil {
		log.Warnf(ctx, "Unable to write containers %s state to disk: %v", ctr.ID(), err)
	}
}
