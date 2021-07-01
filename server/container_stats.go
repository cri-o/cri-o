package server

import (
	"context"
	"path/filepath"
	"time"

	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/server/cri/types"
	crioStorage "github.com/cri-o/cri-o/utils"
	"github.com/pkg/errors"
)

func (s *Server) buildContainerStats(ctx context.Context, stats *types.ContainerStats, container *oci.Container) *types.ContainerStats {
	// TODO: Fix this for other storage drivers. This will only work with overlay.
	if s.ContainerServer.Config().RootConfig.Storage == "overlay" {
		diffDir := filepath.Join(filepath.Dir(container.MountPoint()), "diff")
		bytesUsed, inodeUsed, err := crioStorage.GetDiskUsageStats(diffDir)
		if err != nil {
			log.Warnf(ctx, "Unable to get disk usage for container %s， %s", container.ID(), err)
		}
		stats.WritableLayer = &types.FilesystemUsage{
			Timestamp:  time.Now().UnixNano(),
			FsID:       &types.FilesystemIdentifier{Mountpoint: container.MountPoint()},
			UsedBytes:  &types.UInt64Value{Value: bytesUsed},
			InodesUsed: &types.UInt64Value{Value: inodeUsed},
		}
	}
	return stats
}

// ContainerStats returns stats of the container. If the container does not
// exist, the call returns an error.
func (s *Server) ContainerStats(ctx context.Context, req *types.ContainerStatsRequest) (*types.ContainerStatsResponse, error) {
	container, err := s.GetContainerFromShortID(req.ContainerID)
	if err != nil {
		return nil, err
	}
	sb := s.GetSandbox(container.Sandbox())
	if sb == nil {
		return nil, errors.Errorf("unable to get stats for container %s: sandbox %s not found", container.ID(), container.Sandbox())
	}
	cgroup := sb.CgroupParent()

	stats, err := s.Runtime().ContainerStats(ctx, container, cgroup)
	if err != nil {
		return nil, err
	}

	return &types.ContainerStatsResponse{Stats: s.buildContainerStats(ctx, stats, container)}, nil
}
