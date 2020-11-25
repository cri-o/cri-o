package v1alpha2

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (c *service) ImageFsInfo(
	ctx context.Context, req *pb.ImageFsInfoRequest,
) (*pb.ImageFsInfoResponse, error) {
	return nil, nil
}
