package server

import (
	"context"
	"errors"
	"fmt"

	"go.podman.io/storage"
	"golang.org/x/sync/errgroup"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
)

func (s *Server) stopPodSandbox(ctx context.Context, sb *sandbox.Sandbox) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	stopMutex := sb.StopMutex()
	stopMutex.Lock()
	defer stopMutex.Unlock()

	// Clean up sandbox networking and close its network namespace.
	if err := s.networkStop(ctx, sb); err != nil {
		return err
	}

	if sb.Stopped() {
		log.Infof(ctx, "Stopped pod sandbox (already stopped): %s", sb.ID())
		return nil
	}

	// Calculate the timeout once. Regular containers get most of the timeout,
	// reserving a small amount for infra container shutdown. The infra container
	// will use the full totalTimeout, allowing it to use any time saved from
	// containers stopping earlier than their allocated timeout.
	totalTimeout := stopTimeoutFromContext(ctx)

	const infraReservedTimeout int64 = 1 // 1 second reserved for infra container

	var containerTimeout int64

	if totalTimeout < infraReservedTimeout*2 {
		// If total timeout is too small, split evenly
		containerTimeout = totalTimeout / 2
	} else {
		// Reserve fixed time for infra, give rest to containers
		containerTimeout = totalTimeout - infraReservedTimeout
	}

	errorGroup := &errgroup.Group{}

	for _, ctr := range sb.Containers().List() {
		if ctr.State().Status == oci.ContainerStateStopped {
			continue
		}

		// Because ctr is reused across iterations, all goroutines can end up
		// calling stopContainer on the last container in the list instead of
		// their respective one. We fix that by:
		stopCtr := ctr

		errorGroup.Go(func() error {
			return s.stopContainer(ctx, stopCtr, containerTimeout)
		})
	}
	if err := errorGroup.Wait(); err != nil {
		return fmt.Errorf("stop containers in parallel: %w", err)
	}

	podInfraContainer := sb.InfraContainer()
	if err := s.stopContainer(ctx, podInfraContainer, totalTimeout); err != nil && !errors.Is(err, storage.ErrContainerUnknown) && !errors.Is(err, oci.ErrContainerStopped) {
		return fmt.Errorf("failed to stop infra container for pod sandbox %s: %w", sb.ID(), err)
	}

	if err := sb.RemoveManagedNamespaces(); err != nil {
		return fmt.Errorf("unable to remove managed namespaces: %w", err)
	}

	if err := sb.UnmountShm(ctx); err != nil {
		return err
	}

	if err := s.nri.stopPodSandbox(ctx, sb); err != nil {
		return err
	}

	log.Infof(ctx, "Stopped pod sandbox: %s", sb.ID())
	sb.SetStopped(ctx, true)
	s.generateCRIEvent(ctx, sb.InfraContainer(), types.ContainerEventType_CONTAINER_STOPPED_EVENT)

	return nil
}
