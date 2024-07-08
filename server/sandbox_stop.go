package server

import (
	"errors"
	"fmt"

	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
)

// StopPodSandbox stops the sandbox. If there are any running containers in the
// sandbox, they should be force terminated.
func (s *Server) StopPodSandbox(ctx context.Context, req *types.StopPodSandboxRequest) (*types.StopPodSandboxResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	// platform dependent call
	log.Infof(ctx, "Stopping pod sandbox: %s", req.PodSandboxId)
	sb, err := s.getPodSandboxFromRequest(ctx, req.PodSandboxId)
	if err != nil {
		if errors.Is(err, sandbox.ErrIDEmpty) {
			return nil, err
		}
		if errors.Is(err, errSandboxNotCreated) {
			return nil, fmt.Errorf("StopPodSandbox failed as the sandbox is not created: %s", req.PodSandboxId)
		}

		// If the sandbox isn't found we just return an empty response to adhere
		// the CRI interface which expects to not error out in not found
		// cases.

		log.Warnf(ctx, "Could not get sandbox %s, it's probably been stopped already: %v", req.PodSandboxId, err)
		log.Debugf(ctx, "StopPodSandboxResponse %s", req.PodSandboxId)
		return &types.StopPodSandboxResponse{}, nil
	}
	if err := s.stopPodSandbox(ctx, sb); err != nil {
		return nil, err
	}

	return &types.StopPodSandboxResponse{}, nil
}
