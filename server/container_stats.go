package server

import (
	"github.com/cri-o/cri-o/internal/oci"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func buildContainerStats(stats *oci.ContainerStats, container *oci.Container) *pb.ContainerStats {
	return &pb.ContainerStats{
		Attributes: &pb.ContainerAttributes{
			Id:          container.ID(),
			Metadata:    container.Metadata(),
			Labels:      container.Labels(),
			Annotations: container.Annotations(),
		},
		Cpu: &pb.CpuUsage{
			Timestamp:            stats.SystemNano,
			UsageCoreNanoSeconds: &pb.UInt64Value{Value: stats.CPUNano},
		},
		Memory: &pb.MemoryUsage{
			Timestamp:       stats.SystemNano,
			WorkingSetBytes: &pb.UInt64Value{Value: stats.MemUsage},
		},
		WritableLayer: nil,
	}
}

// ContainerStats returns stats of the container. If the container does not
// exist, the call returns an error.
func (s *Server) ContainerStats(ctx context.Context, req *pb.ContainerStatsRequest) (resp *pb.ContainerStatsResponse, err error) {
	container, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, err
	}

	stats, err := s.Runtime().ContainerStats(container)
	if err != nil {
		return nil, err
	}

	return &pb.ContainerStatsResponse{Stats: buildContainerStats(stats, container)}, nil
}
