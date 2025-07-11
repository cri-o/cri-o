package server

import (
	"context"
	"fmt"
	"time"

	json "github.com/json-iterator/go"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
)

// PodSandboxStatus returns the Status of the PodSandbox.
func (s *Server) PodSandboxStatus(ctx context.Context, req *types.PodSandboxStatusRequest) (*types.PodSandboxStatusResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	sb, err := s.getPodSandboxFromRequest(ctx, req.GetPodSandboxId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find pod %q: %v", req.GetPodSandboxId(), err)
	}

	rStatus := sb.State()

	var linux *types.LinuxPodSandboxStatus
	if sb.NamespaceOptions() != nil {
		linux = &types.LinuxPodSandboxStatus{
			Namespaces: &types.Namespace{
				Options: sb.NamespaceOptions(),
			},
		}
	}

	var containerStatuses []*types.ContainerStatus

	var timestamp int64
	if s.config.EnablePodEvents {
		timestamp = time.Now().UnixNano()

		containerStatuses, err = s.getContainerStatusesFromSandboxID(ctx, req.GetPodSandboxId())
		if err != nil {
			return nil, status.Errorf(codes.Unknown, "could not get container statuses of the sandbox Id %q: %v", req.GetPodSandboxId(), err)
		}
	}

	sandboxID := sb.ID()
	resp := &types.PodSandboxStatusResponse{
		Status: &types.PodSandboxStatus{
			Id:          sandboxID,
			CreatedAt:   sb.CreatedAt().UnixNano(),
			Network:     &types.PodSandboxNetworkStatus{},
			State:       rStatus,
			Labels:      sb.Labels(),
			Annotations: sb.Annotations(),
			Metadata:    sb.Metadata(),
			Linux:       linux,
		},
		ContainersStatuses: containerStatuses,
		Timestamp:          timestamp,
	}

	if len(sb.IPs()) > 0 {
		resp.Status.Network.Ip = sb.IPs()[0]
	}

	if len(sb.IPs()) > 1 {
		resp.Status.Network.AdditionalIps = toPodIPs(sb.IPs()[1:])
	}

	if req.GetVerbose() {
		info, err := createSandboxInfo(sb.InfraContainer())
		if err != nil {
			return nil, fmt.Errorf("creating sandbox info: %w", err)
		}

		resp.Info = info
	}

	return resp, nil
}

func toPodIPs(ips []string) (result []*types.PodIP) {
	for _, ip := range ips {
		result = append(result, &types.PodIP{Ip: ip})
	}

	return result
}

func createSandboxInfo(c *oci.Container) (map[string]string, error) {
	var info any
	if c.Spoofed() {
		info = struct {
			RuntimeSpec spec.Spec `json:"runtimeSpec,omitempty"`
		}{
			c.Spec(),
		}
	} else {
		info = struct {
			Image       string    `json:"image"`
			Pid         int       `json:"pid"`
			RuntimeSpec spec.Spec `json:"runtimeSpec,omitempty"`
		}{
			c.UserRequestedImage(),
			c.State().Pid,
			c.Spec(),
		}
	}

	bytes, err := json.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %v: %w", info, err)
	}

	return map[string]string{"info": string(bytes)}, nil
}
