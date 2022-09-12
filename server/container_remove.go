package server

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (s *Server) RemoveContainer(ctx context.Context, req *types.RemoveContainerRequest) error {
	log.Infof(ctx, "Removing container: %s", req.ContainerId)
	// save container description to print
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerId, err)
	}

	sb := s.getSandbox(c.Sandbox())

	if err := s.removeContainerInPod(ctx, sb, c); err != nil {
		return err
	}

	s.Runtime().UpdateContainerStatus(ctx, c)

	s.ContainerEventsChan <- types.ContainerEventResponse{ContainerId: c.ID(), ContainerEventType: types.ContainerEventType_CONTAINER_DELETED_EVENT, PodSandboxMetadata: s.GetSandbox(c.CRIContainer().PodSandboxId).Metadata()}

	log.Infof(ctx, "Removed container %s: %s", c.ID(), c.Description())
	return nil
}

func (s *Server) removeContainerInPod(ctx context.Context, sb *sandbox.Sandbox, c *oci.Container) error {
	if !sb.Stopped() {
		if err := s.stopContainer(ctx, c, int64(10)); err != nil {
			return fmt.Errorf("failed to stop container for removal")
		}
	}

	if err := s.Runtime().DeleteContainer(ctx, c); err != nil {
		return fmt.Errorf("failed to delete container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
	}

	if err := os.Remove(filepath.Join(s.config.ContainerExitsDir, c.ID())); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove container exit file %s: %w", c.ID(), err)
	}

	c.CleanupConmonCgroup()

	if err := s.StorageRuntimeServer().StopContainer(c.ID()); err != nil && err != storage.ErrContainerUnknown {
		// assume container already umounted
		log.Warnf(ctx, "Failed to stop container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
	}

	if err := s.StorageRuntimeServer().DeleteContainer(c.ID()); err != nil && err != storage.ErrContainerUnknown {
		return fmt.Errorf("failed to delete container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
	}

	s.ReleaseContainerName(c.Name())
	s.removeContainer(c)
	if err := s.CtrIDIndex().Delete(c.ID()); err != nil {
		return fmt.Errorf("failed to delete container %s in pod sandbox %s from index: %v", c.Name(), sb.ID(), err)
	}
	sb.RemoveContainer(c)

	return nil
}
