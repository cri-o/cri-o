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

	allStats := make([]*types.ContainerStats, 0, len(ctrList))
	for _, container := range ctrList {
		sb := s.GetSandbox(container.Sandbox())
		if sb == nil {
			log.Warnf(ctx, "unable to get stats for container %s: sandbox %s not found", container.ID(), container.Sandbox())
			continue
		}
		cgroup := sb.CgroupParent()
		stats, err := s.Runtime().ContainerStats(ctx, container, cgroup)
		if err != nil {
			log.Warnf(ctx, "unable to get stats for container %s: %v", container.ID(), err)
			continue
		}
		response := s.buildContainerStats(ctx, stats, container)
		allStats = append(allStats, response)
	}

	return &types.ListContainerStatsResponse{
		Stats: allStats,
	}, nil
}
