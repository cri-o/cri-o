package server

import (
	"context"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *Server) UpdateRuntimeConfig(
	ctx context.Context, req *types.UpdateRuntimeConfigRequest,
) (*types.UpdateRuntimeConfigResponse, error) {
	return &types.UpdateRuntimeConfigResponse{}, nil
}
