package server

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/gogo/protobuf/proto"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// UpdateContainerResources updates ContainerConfig of the container.
func (s *Server) UpdateContainerResources(ctx context.Context, req *pb.UpdateContainerResourcesRequest) (resp *pb.UpdateContainerResourcesResponse, err error) {
	c, err := s.GetContainerFromShortID(req.GetContainerId())
	if err != nil {
		return nil, err
	}
	state := c.State()
	if !(state.Status == oci.ContainerStateRunning || state.Status == oci.ContainerStateCreated) {
		return nil, fmt.Errorf("container %s is not running or created state: %s", c.ID(), state.Status)
	}

	resources := toOCIResources(req.GetLinux())
	if err := s.Runtime().UpdateContainer(c, resources); err != nil {
		return nil, err
	}
	return &pb.UpdateContainerResourcesResponse{}, nil
}

// toOCIResources converts CRI resource constraints to OCI.
func toOCIResources(r *pb.LinuxContainerResources) *rspec.LinuxResources {
	var (
		shares int64
		swap   int64
		quota  int64
	)
	memory := r.GetMemoryLimitInBytes()
	if cgroupHasMemorySwap() {
		swap = memory
	}
	// only set period or quota if both are configured, or else the runtime will fail
	if r.GetCpuShares() != 0 && r.GetCpuQuota() != 0 {
		shares = r.GetCpuShares()
		quota = r.GetCpuQuota()
	}
	return &rspec.LinuxResources{
		CPU: &rspec.LinuxCPU{
			Shares: proto.Uint64(uint64(shares)),
			Quota:  proto.Int64(quota),
			Period: proto.Uint64(uint64(r.GetCpuPeriod())),
			Cpus:   r.GetCpusetCpus(),
			Mems:   r.GetCpusetMems(),
		},
		Memory: &rspec.LinuxMemory{
			Limit: proto.Int64(memory),
			Swap:  proto.Int64(swap),
		},
		// TODO(runcom): OOMScoreAdj is missing
	}
}
