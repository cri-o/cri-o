package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.podman.io/storage"
	"go.podman.io/storage/pkg/truncindex"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
)

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (s *Server) RemoveContainer(
	ctx context.Context,
	req *types.RemoveContainerRequest,
) (*types.RemoveContainerResponse, error) {
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

		return nil, status.Errorf(
			codes.NotFound,
			"could not find container %q: %v",
			req.GetContainerId(),
			err,
		)
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

// containerExited returns whether the container is stopped, or has an exit file
// which the exit monitor did not handle yet.
func (s *Server) containerExited(c *oci.Container) bool {
	if c.State().Status == oci.ContainerStateStopped {
		return true
	}

	_, err := os.Stat(filepath.Join(s.config.ContainerExitsDir, c.ID()))

	return err == nil
}

func (s *Server) removeContainerInPod(
	ctx context.Context,
	sb *sandbox.Sandbox,
	c *oci.Container,
) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	switch {
	case !sb.Stopped() || !s.containerExited(c):
		// Stopping runs the post-stop cleanup, also for a container which still
		// runs in a stopped sandbox, e.g. after its infra container exited.
		if err := s.stopContainer(ctx, c, stopTimeoutFromContext(ctx)); err != nil {
			if !sb.Stopped() {
				return fmt.Errorf("failed to stop container for removal %w", err)
			}

			// The sandbox is stopped, so remove the container anyway, as before.
			log.Warnf(ctx, "Failed to stop container %s for removal: %v", c.ID(), err)
			c.AwaitPostStopCleanup()
		}
	case !c.PostStopDone():
		// The sandbox may be stopped before the post-stop cleanup of this
		// container ran. Run it here, or wait for it if it is already running.
		hooks := s.hooksRetriever.Get(ctx, sb.RuntimeHandler(), sb.Annotations())
		s.postStopCleanup(ctx, c, sb, hooks)
	default:
		// A cleanup must not run after the container is removed.
		c.AwaitPostStopCleanup()
	}

	if err := s.nri.removeContainer(ctx, sb, c); err != nil {
		log.Warnf(ctx, "NRI container removal failed for container %s of pod %s: %v",
			c.ID(), sb.ID(), err)
	}

	if err := s.ContainerServer.Runtime().DeleteContainer(ctx, c); err != nil {
		return fmt.Errorf(
			"failed to delete container %s in pod sandbox %s: %w",
			c.Name(),
			sb.ID(),
			err,
		)
	}

	if err := os.Remove(
		filepath.Join(s.config.ContainerExitsDir, c.ID()),
	); err != nil &&
		!os.IsNotExist(err) {
		return fmt.Errorf("failed to remove container exit file %s: %w", c.ID(), err)
	}

	c.CleanupConmonCgroup(ctx)

	runtimeSvc, err := s.StorageRuntimeServer(sb)
	if err != nil {
		return fmt.Errorf("failed to get runtime service for container %s: %w", c.Name(), err)
	}

	if err := runtimeSvc.DeleteContainer(
		ctx,
		c.ID(),
	); err != nil &&
		!errors.Is(err, storage.ErrContainerUnknown) {
		return fmt.Errorf(
			"failed to delete container %s in pod sandbox %s: %w",
			c.Name(),
			sb.ID(),
			err,
		)
	}

	s.ReleaseContainerName(ctx, c.Name())
	s.removeContainer(ctx, c)

	if err := s.ContainerServer.CtrIDIndex().Delete(c.ID()); err != nil {
		return fmt.Errorf(
			"failed to delete container %s in pod sandbox %s from index: %w",
			c.Name(),
			sb.ID(),
			err,
		)
	}

	sb.RemoveContainer(ctx, c)

	return nil
}
