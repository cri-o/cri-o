package server

import (
	"fmt"
	"io"

	"github.com/cri-o/cri-o/internal/oci"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// PortForward prepares a streaming endpoint to forward ports from a PodSandbox.
func (s *Server) PortForward(ctx context.Context, req *pb.PortForwardRequest) (resp *pb.PortForwardResponse, err error) {
	resp, err = s.getPortForward(req)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare portforward endpoint")
	}

	return resp, nil
}

func (s StreamService) PortForward(podSandboxID string, port int32, stream io.ReadWriteCloser) error {
	sandboxID, err := s.runtimeServer.PodIDIndex().Get(podSandboxID)
	if err != nil {
		return fmt.Errorf("PodSandbox with ID starting with %s not found: %v", podSandboxID, err)
	}
	c := s.runtimeServer.GetSandboxContainer(sandboxID)

	if c == nil {
		return fmt.Errorf("could not find container for sandbox %q", podSandboxID)
	}

	if err := s.runtimeServer.Runtime().UpdateContainerStatus(c); err != nil {
		return err
	}

	cState := c.State()
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return fmt.Errorf("container is not created or running")
	}

	return s.runtimeServer.Runtime().PortForwardContainer(c, port, stream)
}
