package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containers/storage"
	"github.com/containers/storage/pkg/truncindex"
	"github.com/cri-o/cri-o/internal/lib/sandbox"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (s *Server) RemoveContainer(ctx context.Context, req *types.RemoveContainerRequest) (*types.RemoveContainerResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	log.Infof(ctx, "Removing container: %s", req.ContainerId)
	// save container description to print
	c, err := s.GetContainerFromShortID(ctx, req.ContainerId)
	if err != nil {
		// The RemoveContainer RPC is idempotent, and must not return an error
		// if the container has already been removed. Ref:
		// https://github.com/kubernetes/cri-api/blob/c20fa40/pkg/apis/runtime/v1/api.proto#L74-L75
		if errors.Is(err, truncindex.ErrNotExist) {
			return &types.RemoveContainerResponse{}, nil
		}
		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerId, err)
	}

	sb := s.getSandbox(ctx, c.Sandbox())

	if err := s.removeContainerInPod(ctx, sb, c); err != nil {
		return nil, err
	}

	s.removeSeccompNotifier(ctx, c)

	s.generateCRIEvent(ctx, c, types.ContainerEventType_CONTAINER_DELETED_EVENT)
	log.Infof(ctx, "Removed container %s: %s", c.ID(), c.Description())
	return &types.RemoveContainerResponse{}, nil
}

func (s *Server) removeContainerInPod(ctx context.Context, sb *sandbox.Sandbox, c *oci.Container) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	if !sb.Stopped() {
		if err := s.stopContainer(ctx, c, int64(10)); err != nil {
			return fmt.Errorf("failed to stop container for removal %w", err)
		}
	}

	if err := s.nri.removeContainer(ctx, sb, c); err != nil {
		log.Warnf(ctx, "NRI container removal failed for container %s of pod %s: %v",
			c.ID(), sb.ID(), err)
	}

	if err := s.Runtime().DeleteContainer(ctx, c); err != nil {
		return fmt.Errorf("failed to delete container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
	}

	if err := os.Remove(filepath.Join(s.config.ContainerExitsDir, c.ID())); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove container exit file %s: %w", c.ID(), err)
	}

	c.CleanupConmonCgroup(ctx)

	if err := s.StorageRuntimeServer().StopContainer(ctx, c.ID()); err != nil && err != storage.ErrContainerUnknown {
		// assume container already umounted
		log.Warnf(ctx, "Failed to stop container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
	}

	if err := s.StorageRuntimeServer().DeleteContainer(ctx, c.ID()); err != nil && err != storage.ErrContainerUnknown {
		return fmt.Errorf("failed to delete container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
	}

	s.ReleaseContainerName(ctx, c.Name())
	s.removeContainer(ctx, c)
	if err := s.CtrIDIndex().Delete(c.ID()); err != nil {
		return fmt.Errorf("failed to delete container %s in pod sandbox %s from index: %v", c.Name(), sb.ID(), err)
	}
	sb.RemoveContainer(ctx, c)

	return nil
}
