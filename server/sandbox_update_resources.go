package server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
)

// UpdatePodSandboxResources updates the cgroup resources for the sandbox.
func (s *Server) UpdatePodSandboxResources(ctx context.Context, req *types.UpdatePodSandboxResourcesRequest) (*types.UpdatePodSandboxResourcesResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	sb, err := s.getPodSandboxFromRequest(ctx, req.GetPodSandboxId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find pod %q: %v", req.GetPodSandboxId(), err)
	}

	err = s.nri.updatePodSandbox(ctx, sb, req.GetOverhead(), req.GetResources())
	if err != nil {
		return nil, err
	}

	return &types.UpdatePodSandboxResourcesResponse{}, nil
}
