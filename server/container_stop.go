package server

import (
	"context"
	"errors"
	"fmt"

	"go.podman.io/storage/pkg/truncindex"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
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

	if err := s.ContainerServer.StorageRuntimeServer().StopContainer(ctx, ctr.ID()); err != nil {
		return fmt.Errorf("failed to unmount container %s: %w", ctr.ID(), err)
	}

	if err := s.ContainerStateToDisk(ctx, ctr); err != nil {
		log.Warnf(ctx, "Unable to write containers %s state to disk: %v", ctr.ID(), err)
	}

	if hooks != nil {
		if err := hooks.PostStop(ctx, ctr, sb); err != nil {
			// The hook failure MUST NOT prevent the Pod termination
			log.Errorf(ctx, "Failed to run post-stop hook for container %s: %v", ctr.ID(), err)
		}
	}

	if err := s.nri.stopContainer(ctx, sb, ctr); err != nil {
		return err
	}

	return nil
}
