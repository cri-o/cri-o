package server

import (
	"github.com/cri-o/cri-o/internal/pkg/log"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// ListContainerStats returns stats of all running containers.
func (s *Server) ListContainerStats(ctx context.Context, req *pb.ListContainerStatsRequest) (resp *pb.ListContainerStatsResponse, err error) {
	ctrList, err := s.ContainerServer.ListContainers()
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
		stats, err := s.Runtime().ContainerStats(container)
		if err != nil {
			log.Warnf(ctx, "unable to get stats for container %s", container.ID())
			continue
		}
		response := buildContainerStats(stats, container)
		allStats = append(allStats, response)
	}

	return &pb.ListContainerStatsResponse{
		Stats: allStats,
	}, nil
}
