package server

import (
	"fmt"
	"io"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/client-go/tools/remotecommand"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Exec prepares a streaming endpoint to execute a command in the container.
func (s *Server) Exec(ctx context.Context, req *types.ExecRequest) (*types.ExecResponse, error) {
	resp, err := s.getExec(req)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare exec endpoint: %w", err)
	}

	return resp, nil
}

// Exec endpoint for streaming.Runtime
func (s StreamService) Exec(containerID string, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	c, err := s.runtimeServer.GetContainerFromShortID(containerID)
	if err != nil {
		return status.Errorf(codes.NotFound, "could not find container %q: %v", containerID, err)
	}

	if err := c.IsAlive(); err != nil {
		return status.Errorf(codes.NotFound, "container is not created or running: %v", err)
	}

	return s.runtimeServer.Runtime().ExecContainer(s.ctx, c, cmd, stdin, stdout, stderr, tty, resize)
}
