//go:build linux
// +build linux

package server

import (
	"fmt"

	"github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/runtimehandlerhooks"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"golang.org/x/sync/errgroup"
)

func (s *Server) stopPodSandbox(ctx context.Context, sb *sandbox.Sandbox) error {
	log.Debugf(ctx, "Locking stop mutex")
	stopMutex := sb.StopMutex()
	stopMutex.Lock()
	defer stopMutex.Unlock()

	// Clean up sandbox networking and close its network namespace.
	log.Debugf(ctx, "Stopping network")
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
	log.Debugf(ctx, "Got runtime handler hooks: %v", hooks)

	podInfraContainer := sb.InfraContainer()
	log.Debugf(ctx, "Got pod infra container: %v", podInfraContainer.ID())
	containers := sb.Containers().List()
	containers = append(containers, podInfraContainer)
	log.Debugf(ctx, "Sandbox has %v containers", len(containers))

	const maxWorkers = 128
	var waitGroup errgroup.Group
	for i := 0; i < len(containers); i += maxWorkers {
		max := i + maxWorkers
		if len(containers) < max {
			max = len(containers)
		}
		for _, ctr := range containers[i:max] {
			cStatus := ctr.State()
			log.Debugf(ctx, "Container %v has state %v", ctr.ID(), cStatus)

			if cStatus.Status != oci.ContainerStateStopped {
				if ctr.ID() == podInfraContainer.ID() {
					log.Debugf(ctx, "Container not stopped")
					continue
				}
				c := ctr
				waitGroup.Go(func() error {
					log.Debugf(ctx, "Stopping container and waiting")
					if err := s.StopContainerAndWait(ctx, c, int64(10)); err != nil {
						return fmt.Errorf("failed to stop container for pod sandbox %s: %v", sb.ID(), err)
					}

					log.Debugf(ctx, "Stopping container in storage")
					if err := s.StorageRuntimeServer().StopContainer(c.ID()); err != nil && !errors.Is(err, storage.ErrContainerUnknown) {
						// assume container already umounted
						log.Warnf(ctx, "failed to stop container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
					}

					log.Debugf(ctx, "Writing container state to disk")
					if err := s.ContainerStateToDisk(ctx, c); err != nil {
						return errors.Wrapf(err, "write container %q state do disk", c.Name())
					}
					return nil
				})
			}
			if hooks != nil {
				log.Debugf(ctx, "Running pre stop hook")
				if err := hooks.PreStop(ctx, ctr, sb); err != nil {
					log.Warnf(ctx, "failed to run PreStop hook for container %s in pod sandbox %s: %v", ctr.Name(), sb.ID(), err)
				}
			}
		}
		log.Debugf(ctx, "Waiting for stop wait group to finish")
		if err := waitGroup.Wait(); err != nil {
			return err
		}
	}

	if podInfraContainer != nil {
		podInfraStatus := podInfraContainer.State()
		log.Debugf(ctx, "Checking pod infra status")
		if podInfraStatus.Status != oci.ContainerStateStopped {
			log.Debugf(ctx, "Stopping infra container")
			if err := s.StopContainerAndWait(ctx, podInfraContainer, int64(10)); err != nil {
				return fmt.Errorf("failed to stop infra container for pod sandbox %s: %v", sb.ID(), err)
			}
		}
	}

	log.Debugf(ctx, "Unmounting SHM")
	if err := sb.UnmountShm(); err != nil {
		return err
	}

	log.Debugf(ctx, "Stopping storage container")
	if err := s.StorageRuntimeServer().StopContainer(sb.ID()); err != nil && !errors.Is(err, storage.ErrContainerUnknown) {
		log.Warnf(ctx, "failed to stop sandbox container in pod sandbox %s: %v", sb.ID(), err)
	}
	log.Debugf(ctx, "Writing container state to disk")
	if err := s.ContainerStateToDisk(ctx, podInfraContainer); err != nil {
		log.Warnf(ctx, "error writing pod infra container %q state to disk: %v", podInfraContainer.ID(), err)
	}

	log.Infof(ctx, "Stopped pod sandbox: %s", sb.ID())
	sb.SetStopped(true)

	return nil
}
