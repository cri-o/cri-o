package server

import (
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/server/cri/types"
	"golang.org/x/net/context"
)

// StopPodSandbox stops the sandbox. If there are any running containers in the
// sandbox, they should be force terminated.
func (s *Server) StopPodSandbox(ctx context.Context, req *types.StopPodSandboxRequest) error {
	// platform dependent call
	return s.stopPodSandbox(ctx, req)
}

// stopAllPodSandboxes removes all pod sandboxes
func (s *Server) stopAllPodSandboxes(ctx context.Context) {
	log.Debugf(ctx, "stopAllPodSandboxes")
	for _, sb := range s.ContainerServer.ListSandboxes() {
		pod := &types.StopPodSandboxRequest{
			PodSandboxID: sb.ID(),
		}
		if err := s.StopPodSandbox(ctx, pod); err != nil {
			log.Warnf(ctx, "could not StopPodSandbox %s: %v", sb.ID(), err)
		}
	}
}
