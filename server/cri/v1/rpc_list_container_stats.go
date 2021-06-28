package v1

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ListContainerStats(
	ctx context.Context, req *pb.ListContainerStatsRequest,
) (*pb.ListContainerStatsResponse, error) {
	r := &types.ListContainerStatsRequest{}

	if req.Filter != nil {
		r.Filter = &types.ContainerStatsFilter{
			ID:            req.Filter.Id,
			LabelSelector: req.Filter.LabelSelector,
			PodSandboxID:  req.Filter.PodSandboxId,
		}
	}

	res, err := s.server.ListContainerStats(ctx, r)
	if err != nil {
		return nil, err
	}

	resp := &pb.ListContainerStatsResponse{
		Stats: []*pb.ContainerStats{},
	}

	for _, stat := range res.Stats {
		if stat == nil {
			continue
		}

		resp.Stats = append(resp.Stats, serverContainerStatToCRI(stat))
	}
	return resp, nil
}
