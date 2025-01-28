package server

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/client-go/tools/remotecommand"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
)

// Exec prepares a streaming endpoint to execute a command in the container.
func (s *Server) Exec(ctx context.Context, req *types.ExecRequest) (*types.ExecResponse, error) {
	resp, err := s.getExec(req)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare exec endpoint: %w", err)
	}

	return resp, nil
}

// Exec endpoint for streaming.Runtime.
func (s *StreamService) Exec(ctx context.Context, containerID string, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resizeChan <-chan remotecommand.TerminalSize) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	c, err := s.runtimeServer.ContainerServer.GetContainerFromShortID(ctx, containerID)
	if err != nil {
		return status.Errorf(codes.NotFound, "could not find container %q: %v", containerID, err)
	}

	if err := s.runtimeServer.ContainerServer.Runtime().UpdateContainerStatus(s.ctx, c); err != nil {
		return err
	}

	cState := c.State()
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return errors.New("container is not created or running")
	}

	return s.runtimeServer.ContainerServer.Runtime().ExecContainer(s.ctx, c, cmd, stdin, stdout, stderr, tty, resizeChan)
}
