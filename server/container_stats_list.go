package server

import (
	"github.com/containers/podman/v3/pkg/cgroups"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/server/cri/types"
	"github.com/pkg/errors"
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
			// Because we don't lock, we will get situations where the container was listed, and then
			// its sandbox was deleted before we got to checking its stats.
			// We should not log in this expected situation.
			continue
		}
		cgroup := sb.CgroupParent()
		stats, err := s.Runtime().ContainerStats(ctx, container, cgroup)
		if err != nil {
			// ErrCgroupDeleted is another situation that will happen if the container
			// is deleted from underneath the call to this function.
			if !errors.Is(err, cgroups.ErrCgroupDeleted) {
				// The other errors are much less likely, and possibly useful to hear about.
				log.Warnf(ctx, "Unable to get stats for container %s: %v", container.ID(), err)
			}
			continue
		}
		response := s.buildContainerStats(ctx, stats, container)
		allStats = append(allStats, response)
	}

	return &types.ListContainerStatsResponse{
		Stats: allStats,
	}, nil
}
