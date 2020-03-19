package server

import (
	"fmt"

	"github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	pkgstorage "github.com/cri-o/cri-o/internal/storage"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// RemovePodSandbox deletes the sandbox. If there are any running containers in the
// sandbox, they should be force deleted.
func (s *Server) RemovePodSandbox(ctx context.Context, req *pb.RemovePodSandboxRequest) (resp *pb.RemovePodSandboxResponse, err error) {
	sb, err := s.getPodSandboxFromRequest(req.PodSandboxId)
	if err != nil {
		if err == sandbox.ErrIDEmpty {
			return nil, err
		}

		// If the sandbox isn't found we just return an empty response to adhere
		// the CRI interface which expects to not error out in not found
		// cases.

		resp = &pb.RemovePodSandboxResponse{}
		log.Warnf(ctx, "could not get sandbox %s, it's probably been removed already: %v", req.PodSandboxId, err)
		return resp, nil
	}

	podInfraContainer := sb.InfraContainer()
	containers := sb.Containers().List()
	if podInfraContainer != nil {
		containers = append(containers, podInfraContainer)
	}

	// Delete all the containers in the sandbox
	cs := lib.ContainerServer()
	for _, c := range containers {
		if !sb.Stopped() {
			cState := c.State()
			if cState.Status == oci.ContainerStateCreated || cState.Status == oci.ContainerStateRunning {
				timeout := int64(10)
				if err := cs.Runtime().StopContainer(ctx, c, timeout); err != nil {
					// Assume container is already stopped
					log.Warnf(ctx, "failed to stop container %s: %v", c.Name(), err)
				}
				if err := cs.Runtime().WaitContainerStateStopped(ctx, c); err != nil {
					return nil, fmt.Errorf("failed to get container 'stopped' status %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
				}
			}
		}

		if err := cs.Runtime().DeleteContainer(c); err != nil {
			return nil, fmt.Errorf("failed to delete container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
		}

		if c.ID() == podInfraContainer.ID() {
			continue
		}

		c.CleanupConmonCgroup()

		if err := cs.StorageRuntimeServer().StopContainer(c.ID()); err != nil && err != storage.ErrContainerUnknown {
			// assume container already umounted
			log.Warnf(ctx, "failed to stop container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
		}
		if err := cs.StorageRuntimeServer().DeleteContainer(c.ID()); err != nil && err != storage.ErrContainerUnknown {
			return nil, fmt.Errorf("failed to delete container %s in pod sandbox %s: %v", c.Name(), sb.ID(), err)
		}

		cs.ReleaseContainerName(c.Name())
		s.removeContainer(c)
		if err := cs.CtrIDIndex().Delete(c.ID()); err != nil {
			return nil, fmt.Errorf("failed to delete container %s in pod sandbox %s from index: %v", c.Name(), sb.ID(), err)
		}
		cs.StopMonitoringConmon(c)
	}

	if podInfraContainer != nil {
		s.removeInfraContainer(podInfraContainer)
		podInfraContainer.CleanupConmonCgroup()

		if err := cs.StorageRuntimeServer().StopContainer(sb.ID()); err != nil && errors.Cause(err) != storage.ErrContainerUnknown {
			log.Warnf(ctx, "failed to stop sandbox container in pod sandbox %s: %v", sb.ID(), err)
		}
	}

	if err := sb.UnmountShm(); err != nil {
		return nil, errors.Wrap(err, "unable to unmount SHM")
	}

	if err := cs.StorageRuntimeServer().RemovePodSandbox(sb.ID()); err != nil && err != pkgstorage.ErrInvalidSandboxID {
		return nil, fmt.Errorf("failed to remove pod sandbox %s: %v", sb.ID(), err)
	}
	if s.config.ManageNSLifecycle {
		if err := sb.RemoveManagedNamespaces(); err != nil {
			return nil, errors.Wrap(err, "unable to remove managed namespaces")
		}
	}

	if podInfraContainer != nil {
		cs.ReleaseContainerName(podInfraContainer.Name())
		if err := cs.CtrIDIndex().Delete(podInfraContainer.ID()); err != nil {
			return nil, fmt.Errorf("failed to delete infra container %s in pod sandbox %s from index: %v", podInfraContainer.ID(), sb.ID(), err)
		}
	}

	cs.ReleasePodName(sb.Name())
	if err := s.removeSandbox(sb.ID()); err != nil {
		log.Warnf(ctx, "failed to remove sandbox: %v", err)
	}
	if err := cs.PodIDIndex().Delete(sb.ID()); err != nil {
		return nil, fmt.Errorf("failed to delete pod sandbox %s from index: %v", sb.ID(), err)
	}

	log.Infof(ctx, "removed pod sandbox: %s", sb.ID())
	resp = &pb.RemovePodSandboxResponse{}
	return resp, nil
}
