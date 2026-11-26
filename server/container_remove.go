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
	libconfig "github.com/cri-o/cri-o/pkg/config"
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

func (s *Server) removeContainerInPod(
	ctx context.Context,
	sb *sandbox.Sandbox,
	c *oci.Container,
) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

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
		return fmt.Errorf(
			"failed to delete container %s in pod sandbox %s: %w",
			c.Name(),
			sb.ID(),
			err,
		)
	}

	// Stopping and deleting can both report success while the init process is
	// still running, because both infer the container's liveness from runtime
	// state that may never have been written. Discarding our own record of the
	// container past this point strands that process and its transient systemd
	// scope with nothing left to reconcile them against, so make sure it is gone.
	//
	// VM runtimes are excluded: there state.InitPid is the virtual machine's
	// PID, which the sandbox's other containers share and which outlives any
	// single one of them, so signalling it would tear down unrelated workloads.
	runtimeType, err := s.ContainerServer.Runtime().RuntimeType(c.RuntimeHandler())
	if err != nil {
		// Leave the process alone rather than signal a PID whose ownership we
		// could not establish.
		log.Warnf(
			ctx,
			"Unable to determine runtime type of container %s, not checking for a leftover init process: %v",
			c.ID(),
			err,
		)
	} else if runtimeType != libconfig.RuntimeTypeVM {
		if err := c.KillInitProcess(ctx); err != nil {
			return fmt.Errorf(
				"failed to kill leftover init process for container %s in pod sandbox %s: %w",
				c.Name(),
				sb.ID(),
				err,
			)
		}
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
