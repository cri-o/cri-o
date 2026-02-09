package server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// CheckpointPod checkpoints a pod sandbox.
func (s *Server) CheckpointPod(ctx context.Context, req *types.CheckpointPodRequest) (*types.CheckpointPodResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CheckpointPod not implemented")
}
