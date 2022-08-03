package server

import (
	"fmt"
	"time"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// RemovePodSandbox deletes the sandbox. If there are any running containers in the
// sandbox, they should be force deleted.
func (s *Server) RemovePodSandbox(ctx context.Context, req *types.RemovePodSandboxRequest) error {
	log.Infof(ctx, "Removing pod sandbox: %s", req.PodSandboxId)
	sb, err := s.getPodSandboxFromRequest(req.PodSandboxId)
	if err != nil {
		if err == sandbox.ErrIDEmpty {
			return err
		}
		if err == errSandboxNotCreated {
			return fmt.Errorf("sandbox %s is not yet created", req.PodSandboxId)
		}

		// If the sandbox isn't found we just return an empty response to adhere
		// the CRI interface which expects to not error out in not found
		// cases.
		log.Warnf(ctx, "Could not get sandbox %s, it's probably been removed already: %v", req.PodSandboxId, err)
		return nil
	}
	return s.removePodSandbox(ctx, sb)
}

func (s *Server) removePodSandbox(ctx context.Context, sb *sandbox.Sandbox) error {
	containers := sb.Containers().List()

	// Delete all the containers in the sandbox
	for _, c := range containers {
		if err := s.removeContainerInPod(ctx, sb, c); err != nil {
			return err
		}
	}

	if err := sb.UnmountShm(); err != nil {
		return fmt.Errorf("unable to unmount SHM: %w", err)
	}

	s.removeInfraContainer(sb.InfraContainer())
	if err := s.removeContainerInPod(ctx, sb, sb.InfraContainer()); err != nil {
		return err
	}

	// Cleanup network resources for this pod
	if err := s.networkStop(ctx, sb); err != nil {
		return fmt.Errorf("stop pod network: %w", err)
	}

	if err := sb.RemoveManagedNamespaces(); err != nil {
		return fmt.Errorf("unable to remove managed namespaces: %w", err)
	}

	s.ReleasePodName(sb.Name())
	if err := s.removeSandbox(sb.ID()); err != nil {
		log.Warnf(ctx, "Failed to remove sandbox: %v", err)
	}
	if err := s.PodIDIndex().Delete(sb.ID()); err != nil {
		return fmt.Errorf("failed to delete pod sandbox %s from index: %w", sb.ID(), err)
	}

	if s.config.EventedPLEG {
		if err := s.Runtime().UpdateContainerStatus(ctx, sb.InfraContainer()); err != nil {
			return fmt.Errorf("failed to update the container %s status in the pod sandbox %s: %w", sb.InfraContainer().ID(), sb.ID(), err)
		}
		s.ContainerEventsChan <- types.ContainerEventResponse{ContainerId: sb.ID(), ContainerEventType: types.ContainerEventType_CONTAINER_DELETED_EVENT, CreatedAt: time.Now().UnixNano(), PodSandboxMetadata: sb.Metadata()}
	}

	log.Infof(ctx, "Removed pod sandbox: %s", sb.ID())
	return nil
}
