package server

import (
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/v1alpha2/oci"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// ListContainerStats returns stats of all running containers.
func (s *Server) ListContainerStats(ctx context.Context, req *pb.ListContainerStatsRequest) (*pb.ListContainerStatsResponse, error) {
	ctrList, err := s.ContainerServer.ListContainers(
		func(container *oci.Container) bool {
			return container.StateNoLock().Status != oci.ContainerStateStopped
		},
	)
	if err != nil {
		return nil, err
	}
	filter := req.GetFilter()
	if filter != nil {
		cFilter := &pb.ContainerFilter{
			Id:            req.Filter.Id,
			PodSandboxId:  req.Filter.PodSandboxId,
			LabelSelector: req.Filter.LabelSelector,
		}
		ctrList = s.filterContainerList(ctx, cFilter, ctrList)
	}

	allStats := make([]*pb.ContainerStats, 0, len(ctrList))
	for _, container := range ctrList {
		sb := s.GetSandbox(container.Sandbox())
		if sb == nil {
			log.Warnf(ctx, "unable to get stats for container %s: sandbox %s not found", container.ID(), container.Sandbox())
			continue
		}
		cgroup := sb.CgroupParent()
		stats, err := s.Runtime().ContainerStats(container, cgroup)
		if err != nil {
			log.Warnf(ctx, "unable to get stats for container %s: %v", container.ID(), err)
			continue
		}
		response := s.buildContainerStats(ctx, stats, container)
		allStats = append(allStats, response)
	}

	return &pb.ListContainerStatsResponse{
		Stats: allStats,
	}, nil
}
