package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containers/storage"
	"github.com/containers/storage/pkg/truncindex"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
)

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (s *Server) RemoveContainer(ctx context.Context, req *types.RemoveContainerRequest) (*types.RemoveContainerResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	log.Infof(ctx, "Removing container: %s", req.GetContainerId())
	// save container description to print
	c, err := s.GetContainerFromShortID(ctx, req.GetContainerId())
	if err != nil {
		// The RemoveContainer RPC is idempotent, and must not return an error
		// if the container has already been removed. Ref:
		// https://github.com/kubernetes/cri-api/blob/c20fa40/pkg/apis/runtime/v1/api.proto#L74-L75
		if errors.Is(err, truncindex.ErrNotExist) {
			return &types.RemoveContainerResponse{}, nil
		}

		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.GetContainerId(), err)
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

	// Track if we need to ensure cleanup runs even on failure
	cleanupEnsured := false
	defer func() {
		if !cleanupEnsured {
			// Only log and cleanup if the container has cleanup functions
			if c.HasCleanups() {
				log.Warnf(ctx, "Container removal failed, ensuring artifact cleanup functions run for container %s", c.ID())
				c.Cleanup()
			}
			// Also clean up artifact extract directories from container state
			if extractDirs := c.GetArtifactExtractDirs(); len(extractDirs) > 0 {
				log.Warnf(ctx, "Container removal failed, cleaning up %d artifact extract directories for container %s", len(extractDirs), c.ID())
				for _, extractDir := range extractDirs {
					if err := os.RemoveAll(extractDir); err != nil {
						log.Warnf(ctx, "Failed to clean up artifact extract directory %s for container %s: %v", extractDir, c.ID(), err)
					}
				}
			}
		}
	}()

	if !sb.Stopped() {
		if err := s.stopContainer(ctx, c, stopTimeoutFromContext(ctx)); err != nil {
			return fmt.Errorf("failed to stop container for removal %w", err)
		}
	}

	if err := s.nri.removeContainer(ctx, sb, c); err != nil {
		log.Warnf(ctx, "NRI container removal failed for container %s of pod %s: %v",
			c.ID(), sb.ID(), err)
	}

	if err := s.ContainerServer.Runtime().DeleteContainer(ctx, c); err != nil {
		return fmt.Errorf("failed to delete container %s in pod sandbox %s: %w", c.Name(), sb.ID(), err)
	}

	if err := os.Remove(filepath.Join(s.config.ContainerExitsDir, c.ID())); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove container exit file %s: %w", c.ID(), err)
	}

	c.CleanupConmonCgroup(ctx)

	if err := s.ContainerServer.StorageRuntimeServer().StopContainer(ctx, c.ID()); err != nil && !errors.Is(err, storage.ErrContainerUnknown) {
		// assume container already umounted
		log.Warnf(ctx, "Failed to stop container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
	}

	if err := s.ContainerServer.StorageRuntimeServer().DeleteContainer(ctx, c.ID()); err != nil && !errors.Is(err, storage.ErrContainerUnknown) {
		return fmt.Errorf("failed to delete container %s in pod sandbox %s: %w", c.Name(), sb.ID(), err)
	}

	s.ReleaseContainerName(ctx, c.Name())
	s.removeContainer(ctx, c)

	if err := s.ContainerServer.CtrIDIndex().Delete(c.ID()); err != nil {
		return fmt.Errorf("failed to delete container %s in pod sandbox %s from index: %w", c.Name(), sb.ID(), err)
	}

	sb.RemoveContainer(ctx, c)

	// Call container cleanup to ensure artifact mounts and other resources are properly cleaned up
	// Only run cleanup if the container has cleanup functions
	if c.HasCleanups() {
		c.Cleanup()
	} else {
		log.Debugf(ctx, "Container %s has no cleanup functions", c.ID())
	}

	// Clean up artifact extract directories stored in container state
	// This ensures cleanup happens even after CRI-O restarts
	if extractDirs := c.GetArtifactExtractDirs(); len(extractDirs) > 0 {
		log.Debugf(ctx, "Cleaning up %d artifact extract directories for container %s", len(extractDirs), c.ID())
		for _, extractDir := range extractDirs {
			if err := os.RemoveAll(extractDir); err != nil {
				log.Warnf(ctx, "Failed to clean up artifact extract directory %s for container %s: %v", extractDir, c.ID(), err)
			} else {
				log.Debugf(ctx, "Successfully cleaned up artifact extract directory %s for container %s", extractDir, c.ID())
			}
		}
	}

	cleanupEnsured = true

	return nil
}
