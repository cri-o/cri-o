package v1

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) PodSandboxStats(
	ctx context.Context, req *pb.PodSandboxStatsRequest,
) (*pb.PodSandboxStatsResponse, error) {
	r := &types.PodSandboxStatsRequest{}
	res, err := s.server.PodSandboxStats(ctx, r)
	if err != nil {
		return nil, err
	}
	resp := &pb.PodSandboxStatsResponse{}
	if res.Stats != nil {
		resp.Stats = serverPodSandboxStatToCRI(res.Stats)
	}
	return resp, nil
}
