package server

import (
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/server/cri/types"
	"golang.org/x/net/context"
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
			ID:            req.Filter.ID,
			PodSandboxID:  req.Filter.PodSandboxID,
			LabelSelector: req.Filter.LabelSelector,
		}
		ctrList = s.filterContainerList(ctx, cFilter, ctrList)
	}

	stats, errs := s.CRIStatsForContainers(ctx, ctrList...)
	for _, err := range errs {
		log.Warnf(ctx, "%v", err)
	}
	return &types.ListContainerStatsResponse{
		Stats: stats,
	}, nil
}
