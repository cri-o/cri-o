package server

import (
	"fmt"

	oci "github.com/cri-o/cri-o/internal/oci"
	json "github.com/json-iterator/go"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ContainerStatus returns status of the container.
func (s *Server) ContainerStatus(ctx context.Context, req *types.ContainerStatusRequest) (*types.ContainerStatusResponse, error) {
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerId, err)
	}

	resp := &types.ContainerStatusResponse{
		Status: c.CRIStatus(),
	}

	if req.Verbose {
		info, err := s.createContainerInfo(c)
		if err != nil {
			return nil, fmt.Errorf("creating container info: %w", err)
		}
		resp.Info = info
	}

	return resp, nil
}

func (s *Server) createContainerInfo(container *oci.Container) (map[string]string, error) {
	metadata, err := s.StorageRuntimeServer().GetContainerMetadata(container.ID())
	if err != nil {
		return nil, fmt.Errorf("getting container metadata: %w", err)
	}

	info := struct {
		SandboxID   string    `json:"sandboxID"`
		Pid         int       `json:"pid"`
		RuntimeSpec spec.Spec `json:"runtimeSpec"`
		Privileged  bool      `json:"privileged"`
	}{
		container.Sandbox(),
		container.State().Pid,
		container.Spec(),
		metadata.Privileged,
	}
	bytes, err := json.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %v: %w", info, err)
	}
	return map[string]string{"info": string(bytes)}, nil
}
