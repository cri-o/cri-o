package server

import (
	"fmt"

	"github.com/cri-o/cri-o/server/cri/types"
	"golang.org/x/net/context"
)

// networkNotReadyReason is the reason reported when network is not ready.
const networkNotReadyReason = "NetworkPluginNotReady"

// Status returns the status of the runtime
func (s *Server) Status(ctx context.Context, req *types.StatusRequest) (*types.StatusResponse, error) {
	runtimeCondition := &types.RuntimeCondition{
		Type:   types.RuntimeReady,
		Status: true,
	}
	networkCondition := &types.RuntimeCondition{
		Type:   types.NetworkReady,
		Status: true,
	}

	if err := s.config.CNIPluginReadyOrError(); err != nil {
		networkCondition.Status = false
		networkCondition.Reason = networkNotReadyReason
		networkCondition.Message = fmt.Sprintf("Network plugin returns error: %v", err)
	}

	return &types.StatusResponse{
		Status: &types.RuntimeStatus{
			Conditions: []*types.RuntimeCondition{
				runtimeCondition,
				networkCondition,
			},
		},
	}, nil
}
