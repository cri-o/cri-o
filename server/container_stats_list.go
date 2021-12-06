package server

import (
	"github.com/cri-o/cri-o/internal/oci"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ListContainerStats returns stats of all running containers.
func (s *Server) ListContainerStats(ctx context.Context, req *types.ListContainerStatsRequest) (*types.ListContainerStatsResponse, error) {
	ctrList, err := s.ContainerServer.ListContainers(
		func(container *oci.Container) bool {
			return container.StateNoLock().Status != oci.ContainerStateStopped
		},
	)
	if err != nil {
		return nil, err
	}
	filter := req.Filter
	if filter != nil {
		cFilter := &types.ContainerFilter{
			Id:            req.Filter.Id,
			PodSandboxId:  req.Filter.PodSandboxId,
			LabelSelector: req.Filter.LabelSelector,
		}
		ctrList = s.filterContainerList(ctx, cFilter, ctrList)
	}

	return &types.ListContainerStatsResponse{
		Stats: s.ContainerServer.StatsForContainers(ctrList),
	}, nil
}
