package server

import (
	"context"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
)

func (s *Server) UpdateRuntimeConfig(
	ctx context.Context, req *types.UpdateRuntimeConfigRequest,
) (*types.UpdateRuntimeConfigResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	podCIDR := req.GetRuntimeConfig().GetNetworkConfig().GetPodCidr()
	if podCIDR != "" {
		log.Infof(ctx, "Updating runtime config with pod CIDR: %s", podCIDR)
	}

	return &types.UpdateRuntimeConfigResponse{}, nil
}
