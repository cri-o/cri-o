package v1

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ContainerStats(
	ctx context.Context, req *pb.ContainerStatsRequest,
) (*pb.ContainerStatsResponse, error) {
	r := &types.ContainerStatsRequest{ContainerID: req.ContainerId}
	res, err := s.server.ContainerStats(ctx, r)
	if err != nil {
		return nil, err
	}
	resp := &pb.ContainerStatsResponse{}
	if res.Stats != nil {
		resp.Stats = &pb.ContainerStats{}
		if res.Stats.Attributes != nil {
			resp.Stats.Attributes = &pb.ContainerAttributes{
				Id:          res.Stats.Attributes.ID,
				Labels:      res.Stats.Attributes.Labels,
				Annotations: res.Stats.Attributes.Annotations,
			}
			if res.Stats.Attributes.Metadata != nil {
				resp.Stats.Attributes.Metadata = &pb.ContainerMetadata{
					Name:    res.Stats.Attributes.Metadata.Name,
					Attempt: res.Stats.Attributes.Metadata.Attempt,
				}
			}
		}
		if res.Stats.CPU != nil {
			resp.Stats.Cpu = &pb.CpuUsage{
				Timestamp: res.Stats.CPU.Timestamp,
			}
			if res.Stats.CPU.UsageCoreNanoSeconds != nil {
				resp.Stats.Cpu.UsageCoreNanoSeconds = &pb.UInt64Value{
					Value: res.Stats.CPU.UsageCoreNanoSeconds.Value,
				}
			}
		}
		if res.Stats.Memory != nil {
			resp.Stats.Memory = &pb.MemoryUsage{
				Timestamp: res.Stats.Memory.Timestamp,
			}
			if res.Stats.Memory.WorkingSetBytes != nil {
				resp.Stats.Memory.WorkingSetBytes = &pb.UInt64Value{
					Value: res.Stats.Memory.WorkingSetBytes.Value,
				}
			}
		}
		if res.Stats.WritableLayer != nil {
			resp.Stats.WritableLayer = &pb.FilesystemUsage{
				Timestamp: res.Stats.WritableLayer.Timestamp,
			}
			if res.Stats.WritableLayer.FsID != nil {
				resp.Stats.WritableLayer.FsId = &pb.FilesystemIdentifier{
					Mountpoint: res.Stats.WritableLayer.FsID.Mountpoint,
				}
			}
			if res.Stats.WritableLayer.UsedBytes != nil {
				resp.Stats.WritableLayer.UsedBytes = &pb.UInt64Value{
					Value: res.Stats.WritableLayer.UsedBytes.Value,
				}
			}
			if res.Stats.WritableLayer.InodesUsed != nil {
				resp.Stats.WritableLayer.InodesUsed = &pb.UInt64Value{
					Value: res.Stats.WritableLayer.InodesUsed.Value,
				}
			}
		}
	}
	return resp, nil
}
