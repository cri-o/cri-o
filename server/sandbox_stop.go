package server

import (
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/log"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// StopPodSandbox stops the sandbox. If there are any running containers in the
// sandbox, they should be force terminated.
func (s *Server) StopPodSandbox(ctx context.Context, req *pb.StopPodSandboxRequest) (resp *pb.StopPodSandboxResponse, err error) {
	// platform dependent call
	return s.stopPodSandbox(ctx, req)
}

// stopAllPodSandboxes removes all pod sandboxes
func (s *Server) stopAllPodSandboxes(ctx context.Context) {
	log.Debugf(ctx, "stopAllPodSandboxes")
	for _, sb := range lib.ContainerServer().ListSandboxes() {
		pod := &pb.StopPodSandboxRequest{
			PodSandboxId: sb.ID(),
		}
		if _, err := s.StopPodSandbox(ctx, pod); err != nil {
			log.Warnf(ctx, "could not StopPodSandbox %s: %v", sb.ID(), err)
		}
	}
}
