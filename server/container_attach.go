package server

import (
	"errors"
	"fmt"
	"io"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/client-go/tools/remotecommand"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Attach prepares a streaming endpoint to attach to a running container.
func (s *Server) Attach(ctx context.Context, req *types.AttachRequest) (*types.AttachResponse, error) {
	c, err := s.GetContainerFromShortID(ctx, req.ContainerId)
	if err != nil {
		return nil, fmt.Errorf("could not find container %q: %w", req.ContainerId, err)
	}

	url, err := s.Runtime().ServeAttachContainer(ctx, c, req.Stdin, req.Stdout, req.Stderr)
	if err != nil {
		return nil, fmt.Errorf("could not serve attach for container %q: %w", req.ContainerId, err)
	}
	if url != "" {
		log.Infof(ctx, "Using attach URL from runtime: %v", url)
		return &types.AttachResponse{Url: url}, nil
	}

	resp, err := s.getAttach(req)
	if err != nil {
		return nil, errors.New("unable to prepare attach endpoint")
	}

	return resp, nil
}

// Attach endpoint for streaming.Runtime
func (s StreamService) Attach(ctx context.Context, containerID string, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resizeChan <-chan remotecommand.TerminalSize) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	c, err := s.runtimeServer.GetContainerFromShortID(ctx, containerID)
	if err != nil {
		return status.Errorf(codes.NotFound, "could not find container %q: %v", containerID, err)
	}

	if err := s.runtimeServer.Runtime().UpdateContainerStatus(s.ctx, c); err != nil {
		return err
	}

	cState := c.State()
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return errors.New("container is not created or running")
	}

	return s.runtimeServer.Runtime().AttachContainer(s.ctx, c, inputStream, outputStream, errorStream, tty, resizeChan)
}
