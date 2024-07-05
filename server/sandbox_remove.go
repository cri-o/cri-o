package server

import (
	"errors"
	"fmt"

	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
)

// RemovePodSandbox deletes the sandbox. If there are any running containers in the
// sandbox, they should be force deleted.
func (s *Server) RemovePodSandbox(ctx context.Context, req *types.RemovePodSandboxRequest) (*types.RemovePodSandboxResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	log.Infof(ctx, "Removing pod sandbox: %s", req.PodSandboxId)
	sb, err := s.getPodSandboxFromRequest(ctx, req.PodSandboxId)
	if err != nil {
		if errors.Is(err, sandbox.ErrIDEmpty) {
			return nil, err
		}
		if errors.Is(err, errSandboxNotCreated) {
			return nil, fmt.Errorf("sandbox %s is not yet created", req.PodSandboxId)
		}

		// If the sandbox isn't found we just return an empty response to adhere
		// the CRI interface which expects to not error out in not found
		// cases.
		log.Warnf(ctx, "Could not get sandbox %s, it's probably been removed already: %v", req.PodSandboxId, err)
		return &types.RemovePodSandboxResponse{}, nil
	}
	if err := s.removePodSandbox(ctx, sb); err != nil {
		return nil, err
	}

	return &types.RemovePodSandboxResponse{}, nil
}

func (s *Server) removePodSandbox(ctx context.Context, sb *sandbox.Sandbox) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	containers := sb.Containers().List()

	// Delete all the containers in the sandbox
	for _, c := range containers {
		if err := s.removeContainerInPod(ctx, sb, c); err != nil {
			return err
		}
	}

	if err := sb.UnmountShm(ctx); err != nil {
		return fmt.Errorf("unable to unmount SHM: %w", err)
	}

	s.removeInfraContainer(ctx, sb.InfraContainer())
	if err := s.removeContainerInPod(ctx, sb, sb.InfraContainer()); err != nil {
		return err
	}
	if sb.InfraContainer().Spoofed() {
		if err := s.config.CgroupManager().RemoveSandboxCgroup(sb.CgroupParent(), sb.ID()); err != nil {
			return err
		}
	}

	// Cleanup network resources for this pod
	if err := s.networkStop(ctx, sb); err != nil {
		return fmt.Errorf("stop pod network: %w", err)
	}

	if err := sb.RemoveManagedNamespaces(); err != nil {
		return fmt.Errorf("unable to remove managed namespaces: %w", err)
	}

	s.ReleasePodName(sb.Name())
	if err := s.removeSandbox(ctx, sb.ID()); err != nil {
		log.Warnf(ctx, "Failed to remove sandbox: %v", err)
	}
	if err := s.PodIDIndex().Delete(sb.ID()); err != nil {
		return fmt.Errorf("failed to delete pod sandbox %s from index: %w", sb.ID(), err)
	}
	s.generateCRIEvent(ctx, sb.InfraContainer(), types.ContainerEventType_CONTAINER_DELETED_EVENT)

	if err := s.nri.removePodSandbox(ctx, sb); err != nil {
		log.Warnf(ctx, "NRI pod removal failed for %q: %v", sb.ID(), err)
	}

	log.Infof(ctx, "Removed pod sandbox: %s", sb.ID())
	return nil
}
