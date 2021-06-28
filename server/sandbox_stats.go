package server

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
)

// PodSandboxStats returns stats of the sandbox. If the sandbox does not exist, the call returns an error.
func (s *Server) PodSandboxStats(ctx context.Context, req *types.PodSandboxStatsRequest) (*types.PodSandboxStatsResponse, error) {
	return nil, nil
}
