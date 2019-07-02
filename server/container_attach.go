package server

import (
	"fmt"
	"io"
	"time"

	"github.com/cri-o/cri-o/internal/oci"
	"golang.org/x/net/context"
	"k8s.io/client-go/tools/remotecommand"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// Attach prepares a streaming endpoint to attach to a running container.
func (s *Server) Attach(ctx context.Context, req *pb.AttachRequest) (resp *pb.AttachResponse, err error) {
	const operation = "attach"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	resp, err = s.getAttach(req)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare attach endpoint")
	}

	return resp, nil
}

// Attach endpoint for streaming.Runtime
func (ss StreamService) Attach(containerID string, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	c, err := ss.runtimeServer.GetContainerFromShortID(containerID)
	if err != nil {
		return fmt.Errorf("could not find container %q: %v", containerID, err)
	}

	if err := ss.runtimeServer.Runtime().UpdateContainerStatus(c); err != nil {
		return err
	}

	cState := c.State()
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return fmt.Errorf("container is not created or running")
	}

	return ss.runtimeServer.Runtime().AttachContainer(c, inputStream, outputStream, errorStream, tty, resize)
}
