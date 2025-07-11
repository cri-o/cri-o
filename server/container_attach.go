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

// Attach prepares a streaming endpoint to attach to a running container.
func (s *Server) Attach(ctx context.Context, req *types.AttachRequest) (*types.AttachResponse, error) {
	c, err := s.GetContainerFromShortID(ctx, req.GetContainerId())
	if err != nil {
		return nil, fmt.Errorf("could not find container %q: %w", req.GetContainerId(), err)
	}

	runtimeHandler := s.getSandbox(ctx, c.Sandbox()).RuntimeHandler()
	if streamWebsocket, err := s.Runtime().RuntimeStreamWebsockets(runtimeHandler); err == nil && streamWebsocket {
		log.Debugf(ctx, "Runtime handler %q is configured to use websockets", runtimeHandler)

		url, err := s.Runtime().ServeAttachContainer(ctx, c, req.GetStdin(), req.GetStdout(), req.GetStderr())
		if err != nil {
			return nil, fmt.Errorf("could not serve attach for container %q: %w", req.GetContainerId(), err)
		}

		log.Infof(ctx, "Using attach URL from container monitor")

		return &types.AttachResponse{Url: url}, nil
	}

	resp, err := s.getAttach(req)
	if err != nil {
		return nil, errors.New("unable to prepare attach endpoint")
	}

	return resp, nil
}

// Attach endpoint for streaming.Runtime.
func (s *StreamService) Attach(ctx context.Context, containerID string, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resizeChan <-chan remotecommand.TerminalSize) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	c, err := s.runtimeServer.GetContainerFromShortID(ctx, containerID)
	if err != nil {
		return status.Errorf(codes.NotFound, "could not find container %q: %v", containerID, err)
	}

	if err := s.runtimeServer.ContainerServer.Runtime().UpdateContainerStatus(s.ctx, c); err != nil {
		return err
	}

	cState := c.State()
	if cState.Status != oci.ContainerStateRunning && cState.Status != oci.ContainerStateCreated {
		return errors.New("container is not created or running")
	}

	return s.runtimeServer.ContainerServer.Runtime().AttachContainer(s.ctx, c, inputStream, outputStream, errorStream, tty, resizeChan)
}
