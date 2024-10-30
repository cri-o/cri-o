package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	errorUtils "k8s.io/apimachinery/pkg/util/errors"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
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

	funcs := []func() error{}
	for _, ctr := range sb.Containers().List() {
		if ctr.State().Status == oci.ContainerStateStopped {
			continue
		}

		funcs = append(funcs, func() error {
			return s.stopContainer(ctx, ctr, stopTimeoutFromContext(ctx))
		})
	}
	if err := errorUtils.AggregateGoroutines(funcs...); err != nil {
		return fmt.Errorf("stop containers in parallel: %w", err)
	}

	podInfraContainer := sb.InfraContainer()
	if err := s.stopContainer(ctx, podInfraContainer, stopTimeoutFromContext(ctx)); err != nil && !errors.Is(err, storage.ErrContainerUnknown) && !errors.Is(err, oci.ErrContainerStopped) {
		return fmt.Errorf("failed to stop infra container for pod sandbox %s: %v", sb.ID(), err)
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
