package v1alpha2

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) ListContainerStats(
	ctx context.Context, req *pb.ListContainerStatsRequest,
) (*pb.ListContainerStatsResponse, error) {
	r := &types.ListContainerStatsRequest{}

	if req.Filter != nil {
		r.Filter = &types.ContainerStatsFilter{
			ID:            req.Filter.Id,
			LabelSelector: req.Filter.LabelSelector,
			PodSandboxID:  req.Filter.PodSandboxId,
		}
	}

	res, err := s.server.ListContainerStats(ctx, r)
	if err != nil {
		return nil, err
	}

	resp := &pb.ListContainerStatsResponse{
		Stats: []*pb.ContainerStats{},
	}

	for _, x := range res.Stats {
		if x == nil {
			continue
		}
		stats := &pb.ContainerStats{}

		if x.Attributes != nil {
			stats.Attributes = &pb.ContainerAttributes{
				Id:          x.Attributes.ID,
				Labels:      x.Attributes.Labels,
				Annotations: x.Attributes.Annotations,
			}
			if x.Attributes.Metadata != nil {
				stats.Attributes.Metadata = &pb.ContainerMetadata{
					Name:    x.Attributes.Metadata.Name,
					Attempt: x.Attributes.Metadata.Attempt,
				}
			}
		}
		if x.CPU != nil {
			stats.Cpu = &pb.CpuUsage{
				Timestamp: x.CPU.Timestamp,
			}
			if x.CPU.UsageCoreNanoSeconds != nil {
				stats.Cpu.UsageCoreNanoSeconds = &pb.UInt64Value{
					Value: x.CPU.UsageCoreNanoSeconds.Value,
				}
			}
		}
		if x.Memory != nil {
			stats.Memory = &pb.MemoryUsage{
				Timestamp: x.Memory.Timestamp,
			}
			if x.Memory.WorkingSetBytes != nil {
				stats.Memory.WorkingSetBytes = &pb.UInt64Value{
					Value: x.Memory.WorkingSetBytes.Value,
				}
			}
		}
		if x.WritableLayer != nil {
			stats.WritableLayer = &pb.FilesystemUsage{
				Timestamp: x.WritableLayer.Timestamp,
			}
			if x.WritableLayer.FsID != nil {
				stats.WritableLayer.FsId = &pb.FilesystemIdentifier{
					Mountpoint: x.WritableLayer.FsID.Mountpoint,
				}
			}
			if x.WritableLayer.UsedBytes != nil {
				stats.WritableLayer.UsedBytes = &pb.UInt64Value{
					Value: x.WritableLayer.UsedBytes.Value,
				}
			}
			if x.WritableLayer.InodesUsed != nil {
				stats.WritableLayer.InodesUsed = &pb.UInt64Value{
					Value: x.WritableLayer.InodesUsed.Value,
				}
			}
		}

		resp.Stats = append(resp.Stats, stats)
	}
	return resp, nil
}
