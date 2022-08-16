package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) CheckpointContainer(ctx context.Context, req *pb.CheckpointContainerRequest) (*pb.CheckpointContainerResponse, error) {
	// Spoofing the CheckpointContainer method to return nil response until actual support code is added.
	return nil, nil
}
