package v1

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ListPodSandboxStats(
	ctx context.Context, req *pb.ListPodSandboxStatsRequest,
) (*pb.ListPodSandboxStatsResponse, error) {
	r := &types.ListPodSandboxStatsRequest{}

	if req.Filter != nil {
		r.Filter = &types.PodSandboxStatsFilter{
			ID:            req.Filter.Id,
			LabelSelector: req.Filter.LabelSelector,
		}
	}

	res, err := s.server.ListPodSandboxStats(ctx, r)
	if err != nil {
		return nil, err
	}

	resp := &pb.ListPodSandboxStatsResponse{
		Stats: []*pb.PodSandboxStats{},
	}

	for _, stat := range res.Stats {
		if stat == nil {
			continue
		}

		resp.Stats = append(resp.Stats, serverPodSandboxStatToCRI(stat))
	}
	return resp, nil
}
