package server

import (
	"errors"
	"fmt"

	"github.com/containers/storage"
	"golang.org/x/net/context"
	"golang.org/x/sync/errgroup"
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

	podInfraContainer := sb.InfraContainer()
	containers := sb.Containers().List()
	containers = append(containers, podInfraContainer)

	const maxWorkers = 128
	var waitGroup errgroup.Group
	for i := 0; i < len(containers); i += maxWorkers {
		maxContainers := i + maxWorkers
		if len(containers) < maxContainers {
			maxContainers = len(containers)
		}
		for _, ctr := range containers[i:maxContainers] {
			cStatus := ctr.State()
			if cStatus.Status != oci.ContainerStateStopped {
				if ctr.ID() == podInfraContainer.ID() {
					continue
				}
				c := ctr
				waitGroup.Go(func() error {
					if err := s.stopContainer(ctx, c, stopTimeoutFromContext(ctx)); err != nil {
						return fmt.Errorf("failed to stop container for pod sandbox %s: %w", sb.ID(), err)
					}
					return nil
				})
			}
		}
		if err := waitGroup.Wait(); err != nil {
			return err
		}
	}

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
