package server

import (
	"errors"
	"fmt"

	"github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/runtimehandlerhooks"
	"golang.org/x/net/context"
	"golang.org/x/sync/errgroup"
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

	// Get high-performance runtime hook to trigger preStop step for each container
	hooks, err := runtimehandlerhooks.GetRuntimeHandlerHooks(ctx, &s.config, sb.RuntimeHandler(), sb.Annotations())
	if err != nil {
		return fmt.Errorf("failed to get runtime handler %q hooks", sb.RuntimeHandler())
	}

	podInfraContainer := sb.InfraContainer()
	containers := sb.Containers().List()
	containers = append(containers, podInfraContainer)

	const maxWorkers = 128
	var waitGroup errgroup.Group
	for i := 0; i < len(containers); i += maxWorkers {
		max := i + maxWorkers
		if len(containers) < max {
			max = len(containers)
		}
		for _, ctr := range containers[i:max] {
			cStatus := ctr.State()
			if cStatus.Status != oci.ContainerStateStopped {
				if ctr.ID() == podInfraContainer.ID() {
					continue
				}
				c := ctr
				waitGroup.Go(func() error {
					if err := s.stopContainer(ctx, c, int64(10)); err != nil {
						return fmt.Errorf("failed to stop container for pod sandbox %s: %v", sb.ID(), err)
					}
					if err := s.nri.stopContainer(ctx, sb, c); err != nil {
						return err
					}
					return nil
				})
			}
			if hooks != nil {
				if err := hooks.PreStop(ctx, ctr, sb); err != nil {
					log.Warnf(ctx, "Failed to run PreStop hook for container %s in pod sandbox %s: %v", ctr.Name(), sb.ID(), err)
				}
			}
		}
		if err := waitGroup.Wait(); err != nil {
			return err
		}
	}

	if err := s.stopContainer(ctx, podInfraContainer, int64(10)); err != nil && !errors.Is(err, storage.ErrContainerUnknown) && !errors.Is(err, oci.ErrContainerStopped) {
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
