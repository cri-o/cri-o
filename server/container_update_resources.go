package server

import (
	"github.com/gogo/protobuf/proto"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

// UpdateContainerResources updates ContainerConfig of the container.
func (s *Server) UpdateContainerResources(ctx context.Context, req *pb.UpdateContainerResourcesRequest) (*pb.UpdateContainerResourcesResponse, error) {
	c, err := s.GetContainerFromRequest(req.GetContainerId())
	if err != nil {
		return nil, err
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
