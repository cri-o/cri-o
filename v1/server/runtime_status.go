package server

import (
	"fmt"

	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// networkNotReadyReason is the reason reported when network is not ready.
const networkNotReadyReason = "NetworkPluginNotReady"

// Status returns the status of the runtime
func (s *Server) Status(ctx context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	runtimeCondition := &pb.RuntimeCondition{
		Type:   pb.RuntimeReady,
		Status: true,
	}
	networkCondition := &pb.RuntimeCondition{
		Type:   pb.NetworkReady,
		Status: true,
	}

	if err := s.config.CNIPlugin().Status(); err != nil {
		networkCondition.Status = false
		networkCondition.Reason = networkNotReadyReason
		networkCondition.Message = fmt.Sprintf("Network plugin returns error: %v", err)
	}

	return &pb.StatusResponse{
		Status: &pb.RuntimeStatus{
			Conditions: []*pb.RuntimeCondition{
				runtimeCondition,
				networkCondition,
			},
		},
	}, nil
}
