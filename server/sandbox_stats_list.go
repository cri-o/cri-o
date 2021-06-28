package server

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
)

// ListPodSandboxStats returns stats of all sandboxes.
func (s *Server) ListPodSandboxStats(ctx context.Context, req *types.ListPodSandboxStatsRequest) (*types.ListPodSandboxStatsResponse, error) {
	return nil, nil
}
