package v1alpha2

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) UpdateContainerResources(
	ctx context.Context, req *pb.UpdateContainerResourcesRequest,
) (*pb.UpdateContainerResourcesResponse, error) {
	r := &types.UpdateContainerResourcesRequest{
		ContainerID: req.ContainerId,
	}
	if req.Linux != nil {
		r.Linux = &types.LinuxContainerResources{
			CPUPeriod:          req.Linux.CpuPeriod,
			CPUQuota:           req.Linux.CpuQuota,
			CPUShares:          req.Linux.CpuShares,
			MemoryLimitInBytes: req.Linux.MemoryLimitInBytes,
			OomScoreAdj:        req.Linux.OomScoreAdj,
			CPUsetCPUs:         req.Linux.CpusetCpus,
			CPUsetMems:         req.Linux.CpusetMems,
		}
		hugePageLimits := []*types.HugepageLimit{}
		for _, x := range req.Linux.HugepageLimits {
			hugePageLimits = append(hugePageLimits, &types.HugepageLimit{
				PageSize: x.PageSize,
				Limit:    x.Limit,
			})
		}
		r.Linux.HugepageLimits = hugePageLimits
	}

	if err := s.server.UpdateContainerResources(ctx, r); err != nil {
		return nil, err
	}

	return &pb.UpdateContainerResourcesResponse{}, nil
}
