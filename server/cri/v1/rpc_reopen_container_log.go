package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *service) ReopenContainerLog(
	ctx context.Context, req *pb.ReopenContainerLogRequest,
) (*pb.ReopenContainerLogResponse, error) {
	return nil, nil
}
