package server

import (
	"fmt"
	"io"

	"github.com/cri-o/cri-o/internal/oci"
	"golang.org/x/net/context"
	"k8s.io/client-go/tools/remotecommand"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// Exec prepares a streaming endpoint to execute a command in the container.
func (s *Server) Exec(ctx context.Context, req *pb.ExecRequest) (resp *pb.ExecResponse, err error) {
	resp, err = s.getExec(req)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare exec endpoint: %v", err)
	}

	return resp, nil
}

// Exec endpoint for streaming.Runtime
func (s StreamService) Exec(containerID string, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	c, err := s.runtimeServer.GetContainerFromShortID(containerID)
	if err != nil {
		return fmt.Errorf("could not find container %q: %v", containerID, err)
	}

	if err := s.runtimeServer.Runtime().UpdateContainerStatus(c); err != nil {
		return err
	}

	cState := c.State()
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return fmt.Errorf("container is not created or running")
	}

	return s.runtimeServer.Runtime().ExecContainer(c, cmd, stdin, stdout, stderr, tty, resize)
}
