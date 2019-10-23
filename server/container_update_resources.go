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
	return &rspec.LinuxResources{
		CPU: &rspec.LinuxCPU{
			Shares: proto.Uint64(uint64(r.GetCpuShares())),
			Quota:  proto.Int64(r.GetCpuQuota()),
			Period: proto.Uint64(uint64(r.GetCpuPeriod())),
			Cpus:   r.GetCpusetCpus(),
			Mems:   r.GetCpusetMems(),
		},
		Memory: &rspec.LinuxMemory{
			Limit: proto.Int64(r.GetMemoryLimitInBytes()),
		},
		// TODO(runcom): OOMScoreAdj is missing
	}
}
