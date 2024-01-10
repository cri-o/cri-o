package server

import (
	"fmt"
	"strings"

	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/gogo/protobuf/proto"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/utils/cpuset"

	"golang.org/x/net/context"
)

// UpdateContainerResources updates ContainerConfig of the container.
func (s *Server) UpdateContainerResources(ctx context.Context, req *types.UpdateContainerResourcesRequest) (*types.UpdateContainerResourcesResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	c, err := s.GetContainerFromShortID(ctx, req.ContainerId)
	if err != nil {
		return nil, err
	}
	state := c.State()
	if !(state.Status == oci.ContainerStateRunning || state.Status == oci.ContainerStateCreated) {
		return nil, fmt.Errorf("container %s is not running or created state: %s", c.ID(), state.Status)
	}

	if req.Linux != nil {
		if err := reapplySharedCPUs(c, req); err != nil {
			return nil, err
		}
		updated, err := s.nri.updateContainer(ctx, c, req.Linux)
		if err != nil {
			return nil, err
		}
		if updated == nil {
			updated = req.Linux
		}
		resources := toOCIResources(updated)
		if err := s.Runtime().UpdateContainer(ctx, c, resources); err != nil {
			return nil, err
		}

		// update memory store with updated resources
		s.UpdateContainerLinuxResources(c, resources)

		if err := s.nri.postUpdateContainer(ctx, c); err != nil {
			log.Errorf(ctx, "NRI container post-update failed: %v", err)
		}
	}

	return &types.UpdateContainerResourcesResponse{}, nil
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

// reapplySharedCPUs appends shared CPUs and update the quota to handle CPUManager
func reapplySharedCPUs(c *oci.Container, req *types.UpdateContainerResourcesRequest) error {
	const sharedCPUsEnvVar = "OPENSHIFT_SHARED_CPUS"
	var sharedCpus string

	if c.Spec().Process == nil {
		return nil
	}
	for _, env := range c.Spec().Process.Env {
		keyAndValue := strings.Split(env, "=")
		if keyAndValue[0] == sharedCPUsEnvVar {
			sharedCpus = keyAndValue[1]
		}
	}
	// nothing to do
	if sharedCpus == "" {
		return nil
	}
	shared, err := cpuset.Parse(sharedCpus)
	if err != nil {
		return err
	}
	isolated, err := cpuset.Parse(req.Linux.CpusetCpus)
	if err != nil {
		return err
	}
	req.Linux.CpusetCpus = isolated.Union(shared).String()
	return nil
}
