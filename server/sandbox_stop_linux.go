package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/containers/storage"
	errorUtils "k8s.io/apimachinery/pkg/util/errors"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubeletTypes "k8s.io/kubelet/pkg/types"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/linklogs"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	ann "github.com/cri-o/cri-o/pkg/annotations"
)

func (s *Server) stopPodSandbox(ctx context.Context, sb *sandbox.Sandbox) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	stopMutex := sb.StopMutex()
	stopMutex.Lock()
	defer stopMutex.Unlock()

	// Unlink logs if they were linked
	sbAnnotations := sb.Annotations()
	if emptyDirVolName, ok := sbAnnotations[ann.LinkLogsAnnotation]; ok {
		if err := linklogs.UnmountPodLogs(ctx, sb.Labels()[kubeletTypes.KubernetesPodUIDLabel], emptyDirVolName); err != nil {
			log.Warnf(ctx, "Failed to unlink logs: %v", err)
		}
	}

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
	if err := s.stopContainer(ctx, podInfraContainer, stopTimeoutFromContext(ctx)); err != nil && !errors.Is(err, storage.ErrContainerUnknown) {
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

	if podInfraContainer.Spoofed() {
		// event generation would be needed in case of a spoofed infra container where there is no
		// exit process that hits the handleExit() code.
		s.generateCRIEvent(ctx, sb.InfraContainer(), types.ContainerEventType_CONTAINER_STOPPED_EVENT)
	}

	return nil
}
