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

	for _, stat := range res.Stats {
		if stat == nil {
			continue
		}

		resp.Stats = append(resp.Stats, serverContainerStatToCRI(stat))
	}
	return resp, nil
}

func serverContainerStatToCRI(from *types.ContainerStats) *pb.ContainerStats {
	to := &pb.ContainerStats{}

	if from.Attributes != nil {
		to.Attributes = &pb.ContainerAttributes{
			Id:          from.Attributes.ID,
			Labels:      from.Attributes.Labels,
			Annotations: from.Attributes.Annotations,
		}
		if from.Attributes.Metadata != nil {
			to.Attributes.Metadata = &pb.ContainerMetadata{
				Name:    from.Attributes.Metadata.Name,
				Attempt: from.Attributes.Metadata.Attempt,
			}
		}
	}
	if from.CPU != nil {
		to.Cpu = &pb.CpuUsage{
			Timestamp: from.CPU.Timestamp,
		}
		if from.CPU.UsageCoreNanoSeconds != nil {
			to.Cpu.UsageCoreNanoSeconds = &pb.UInt64Value{
				Value: from.CPU.UsageCoreNanoSeconds.Value,
			}
		}
	}
	if from.Memory != nil {
		to.Memory = &pb.MemoryUsage{
			Timestamp: from.Memory.Timestamp,
		}
		if from.Memory.WorkingSetBytes != nil {
			to.Memory.WorkingSetBytes = &pb.UInt64Value{
				Value: from.Memory.WorkingSetBytes.Value,
			}
		}
	}
	if from.WritableLayer != nil {
		to.WritableLayer = &pb.FilesystemUsage{
			Timestamp: from.WritableLayer.Timestamp,
		}
		if from.WritableLayer.FsID != nil {
			to.WritableLayer.FsId = &pb.FilesystemIdentifier{
				Mountpoint: from.WritableLayer.FsID.Mountpoint,
			}
		}
		if from.WritableLayer.UsedBytes != nil {
			to.WritableLayer.UsedBytes = &pb.UInt64Value{
				Value: from.WritableLayer.UsedBytes.Value,
			}
		}
		if from.WritableLayer.InodesUsed != nil {
			to.WritableLayer.InodesUsed = &pb.UInt64Value{
				Value: from.WritableLayer.InodesUsed.Value,
			}
		}
	}
	return to
}
