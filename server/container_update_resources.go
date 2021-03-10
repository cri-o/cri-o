package server

import (
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/gogo/protobuf/proto"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// UpdateContainerResources updates ContainerConfig of the container.
func (s *Server) UpdateContainerResources(ctx context.Context, req *types.UpdateContainerResourcesRequest) error {
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return err
	}

	if err := c.IsAlive(); err != nil {
		return errors.Errorf("container is not created or running: %v", err)
	}

	if req.Linux != nil {
		resources := toOCIResources(req.Linux)
		if err := s.Runtime().UpdateContainer(ctx, c, resources); err != nil {
			return err
		}

		// update memory store with updated resources
		s.UpdateContainerLinuxResources(c, resources)
	}

	return nil
}

// toOCIResources converts CRI resource constraints to OCI.
func toOCIResources(r *types.LinuxContainerResources) *rspec.LinuxResources {
	update := rspec.LinuxResources{
		// TODO(runcom): OOMScoreAdj is missing
		CPU: &rspec.LinuxCPU{
			Cpus: r.CpusetCpus,
			Mems: r.CpusetMems,
		},
		Memory: &rspec.LinuxMemory{},
	}
	if r.CpuShares != 0 {
		update.CPU.Shares = proto.Uint64(uint64(r.CpuShares))
	}
	if r.CpuPeriod != 0 {
		update.CPU.Period = proto.Uint64(uint64(r.CpuPeriod))
	}
	if r.CpuQuota != 0 {
		update.CPU.Quota = proto.Int64(r.CpuQuota)
	}

	memory := r.MemoryLimitInBytes
	if memory != 0 {
		update.Memory.Limit = proto.Int64(memory)

		if node.CgroupHasMemorySwap() {
			update.Memory.Swap = proto.Int64(memory)
		}
	}
	return &update
}
