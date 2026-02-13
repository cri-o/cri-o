package server

import (
	"context"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
)

// ListContainerStats returns stats of all running containers.
func (s *Server) ListContainerStats(ctx context.Context, req *types.ListContainerStatsRequest) (*types.ListContainerStatsResponse, error) {
	stats, err := s.listContainerStats(ctx, req.GetFilter())
	if err != nil {
		return nil, err
	}

	return &types.ListContainerStatsResponse{
		Stats: stats,
	}, nil
}

// StreamContainerStats returns a stream of container stats.
func (s *Server) StreamContainerStats(req *types.StreamContainerStatsRequest, stream types.RuntimeService_StreamContainerStatsServer) error {
	ctx := stream.Context()

	stats, err := s.listContainerStats(ctx, req.GetFilter())
	if err != nil {
		return err
	}

	for _, stat := range stats {
		if err := stream.Send(&types.StreamContainerStatsResponse{
			ContainerStats: stat,
		}); err != nil {
			return err
		}
	}

	return nil
}

// listContainerStats returns stats for containers matching the filter.
func (s *Server) listContainerStats(ctx context.Context, filter *types.ContainerStatsFilter) ([]*types.ContainerStats, error) {
	ctrList, err := s.ContainerServer.ListContainers(
		func(container *oci.Container) bool {
			return container.StateNoLock().Status != oci.ContainerStateStopped
		},
	)
	if err != nil {
		return nil, err
	}

	if filter != nil {
		cFilter := &types.ContainerFilter{
			Id:            filter.GetId(),
			PodSandboxId:  filter.GetPodSandboxId(),
			LabelSelector: filter.GetLabelSelector(),
		}
		ctrList = s.filterContainerList(ctx, cFilter, ctrList)

		filteredCtrList := []*oci.Container{}

		for _, ctr := range ctrList {
			if filterContainer(ctr.CRIContainer(), cFilter) {
				filteredCtrList = append(filteredCtrList, ctr)
			}
		}

		ctrList = filteredCtrList
	}

	return s.StatsForContainers(ctrList), nil
}
