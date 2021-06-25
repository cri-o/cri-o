package v1

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ContainerStats(
	ctx context.Context, req *pb.ContainerStatsRequest,
) (*pb.ContainerStatsResponse, error) {
	r := &types.ContainerStatsRequest{ContainerID: req.ContainerId}
	res, err := s.server.ContainerStats(ctx, r)
	if err != nil {
		return nil, err
	}
	resp := &pb.ContainerStatsResponse{}
	if res.Stats != nil {
		resp.Stats = serverContainerStatToCRI(res.Stats)
	}
	return resp, nil
}
