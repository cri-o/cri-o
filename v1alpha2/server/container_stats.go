package server

import (
	"path/filepath"

	"github.com/cri-o/cri-o/internal/log"
	crioStorage "github.com/cri-o/cri-o/utils"
	oci "github.com/cri-o/cri-o/v1alpha2/oci"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *Server) buildContainerStats(ctx context.Context, stats *oci.ContainerStats, container *oci.Container) *pb.ContainerStats {
	// TODO: Fix this for other storage drivers. This will only work with overlay.
	var writableLayer *pb.FilesystemUsage
	if s.ContainerServer.Config().RootConfig.Storage == "overlay" {
		diffDir := filepath.Join(filepath.Dir(container.MountPoint()), "diff")
		bytesUsed, inodeUsed, err := crioStorage.GetDiskUsageStats(diffDir)
		if err != nil {
			log.Warnf(ctx, "unable to get disk usage for container %s， %s", container.ID(), err)
		}
		writableLayer = &pb.FilesystemUsage{
			Timestamp:  stats.SystemNano,
			FsId:       &pb.FilesystemIdentifier{Mountpoint: container.MountPoint()},
			UsedBytes:  &pb.UInt64Value{Value: bytesUsed},
			InodesUsed: &pb.UInt64Value{Value: inodeUsed},
		}
	}
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
			WorkingSetBytes: &pb.UInt64Value{Value: stats.WorkingSetBytes},
		},
		WritableLayer: writableLayer,
	}
}

// ContainerStats returns stats of the container. If the container does not
// exist, the call returns an error.
func (s *Server) ContainerStats(ctx context.Context, req *pb.ContainerStatsRequest) (*pb.ContainerStatsResponse, error) {
	container, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, err
	}
	sb := s.GetSandbox(container.Sandbox())
	if sb == nil {
		return nil, errors.Errorf("unable to get stats for container %s: sandbox %s not found", container.ID(), container.Sandbox())
	}
	cgroup := sb.CgroupParent()

	stats, err := s.Runtime().ContainerStats(container, cgroup)
	if err != nil {
		return nil, err
	}

	return &pb.ContainerStatsResponse{Stats: s.buildContainerStats(ctx, stats, container)}, nil
}
