//go:build !linux

package server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *Server) checkpointPod(context.Context, *types.CheckpointPodRequest) (*types.CheckpointPodResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Pod checkpoint is only supported on Linux")
}

func (s *Server) restorePod(context.Context, *types.RestorePodRequest) (*types.RestorePodResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Pod restore is only supported on Linux")
}

func (s *Server) recoverPodCheckpoints(context.Context) error {
	return nil
}

func (s *Server) recoverPodRestores(context.Context) error {
	return nil
}
