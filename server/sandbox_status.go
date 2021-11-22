package server

import (
	"github.com/cri-o/cri-o/internal/oci"
	json "github.com/json-iterator/go"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// PodSandboxStatus returns the Status of the PodSandbox.
func (s *Server) PodSandboxStatus(ctx context.Context, req *types.PodSandboxStatusRequest) (*types.PodSandboxStatusResponse, error) {
	sb, err := s.getPodSandboxFromRequest(req.PodSandboxId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find pod %q: %v", req.PodSandboxId, err)
	}

	rStatus := types.PodSandboxState_SANDBOX_NOTREADY
	if sb.Ready(true) {
		rStatus = types.PodSandboxState_SANDBOX_READY
	}

	var linux *types.LinuxPodSandboxStatus
	if sb.NamespaceOptions() != nil {
		linux = &types.LinuxPodSandboxStatus{
			Namespaces: &types.Namespace{
				Options: sb.NamespaceOptions(),
			},
		}
	}

	sandboxID := sb.ID()
	resp := &types.PodSandboxStatusResponse{
		Status: &types.PodSandboxStatus{
			Id:          sandboxID,
			CreatedAt:   sb.CreatedAt(),
			Network:     &types.PodSandboxNetworkStatus{},
			State:       rStatus,
			Labels:      sb.Labels(),
			Annotations: sb.Annotations(),
			Metadata:    sb.Metadata(),
			Linux:       linux,
		},
	}

	if len(sb.IPs()) > 0 {
		resp.Status.Network.Ip = sb.IPs()[0]
	}
	if len(sb.IPs()) > 1 {
		resp.Status.Network.AdditionalIps = toPodIPs(sb.IPs()[1:])
	}

	if req.Verbose {
		info, err := createSandboxInfo(sb.InfraContainer())
		if err != nil {
			return nil, errors.Wrap(err, "creating sandbox info")
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
	var info interface{}
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
			c.Image(),
			c.State().Pid,
			c.Spec(),
		}
	}
	bytes, err := json.Marshal(info)
	if err != nil {
		return nil, errors.Wrapf(err, "marshal data: %v", info)
	}
	return map[string]string{"info": string(bytes)}, nil
}
