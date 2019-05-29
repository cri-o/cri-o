package server

import (
	"fmt"
	"time"

	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// networkNotReadyReason is the reason reported when network is not ready.
const networkNotReadyReason = "NetworkPluginNotReady"

// Status returns the status of the runtime
func (s *Server) Status(ctx context.Context, req *pb.StatusRequest) (resp *pb.StatusResponse, err error) {
	const operation = "status"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	runtimeCondition := &pb.RuntimeCondition{
		Type:   pb.RuntimeReady,
		Status: true,
	}
	networkCondition := &pb.RuntimeCondition{
		Type:   pb.NetworkReady,
		Status: true,
	}

	if err := s.netPlugin.Status(); err != nil {
		networkCondition.Status = false
		networkCondition.Reason = networkNotReadyReason
		networkCondition.Message = fmt.Sprintf("Network plugin returns error: %v", err)
	}

	cgroupManager := &pb.RuntimeCondition{
		Type:    "CgroupManager",
		Status:  true,
		Message: s.config.CgroupManager,
	}

	resp = &pb.StatusResponse{
		Status: &pb.RuntimeStatus{
			Conditions: []*pb.RuntimeCondition{
				runtimeCondition,
				networkCondition,
				cgroupManager,
			},
		},
	}

	return resp, nil
}
