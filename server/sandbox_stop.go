package server

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// StopPodSandbox stops the sandbox. If there are any running containers in the
// sandbox, they should be force terminated.
func (s *Server) StopPodSandbox(ctx context.Context, req *types.StopPodSandboxRequest) error {
	// platform dependent call
	log.Infof(ctx, "Stopping pod sandbox: %s", req.PodSandboxId)
	sb, err := s.getPodSandboxFromRequest(req.PodSandboxId)
	if err != nil {
		if err == sandbox.ErrIDEmpty {
			return err
		}
		if err == errSandboxNotCreated {
			return fmt.Errorf("StopPodSandbox failed as the sandbox is not created: %s", req.PodSandboxId)
		}

		// If the sandbox isn't found we just return an empty response to adhere
		// the CRI interface which expects to not error out in not found
		// cases.

		log.Warnf(ctx, "Could not get sandbox %s, it's probably been stopped already: %v", req.PodSandboxId, err)
		log.Debugf(ctx, "StopPodSandboxResponse %s", req.PodSandboxId)
		return nil
	}
	return s.stopPodSandbox(ctx, sb)
}
