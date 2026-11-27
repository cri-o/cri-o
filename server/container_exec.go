package server

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/cri-streaming/pkg/streaming/remotecommand"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
)

// execStatus translates an exec error into the CRI status the kubelet expects.
func execStatus(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, oci.ErrContainerNotRunning) {
		return status.Errorf(codes.NotFound, "container is not created or running")
	}

	var stateUnknown *oci.ErrContainerStateUnknown
	if !errors.As(err, &stateUnknown) {
		return err
	}

	code := codes.Unavailable

	switch {
	case errors.Is(stateUnknown, context.DeadlineExceeded) ||
		status.Code(stateUnknown) == codes.DeadlineExceeded:
		code = codes.DeadlineExceeded
	case errors.Is(stateUnknown, context.Canceled) ||
		status.Code(stateUnknown) == codes.Canceled:
		code = codes.Canceled
	}

	return status.Errorf(
		code, "unable to determine whether container is running: %v", stateUnknown.Err,
	)
}

// Exec prepares a streaming endpoint to execute a command in the container.
func (s *Server) Exec(ctx context.Context, req *types.ExecRequest) (*types.ExecResponse, error) {
	c, err := s.GetContainerFromShortID(ctx, req.GetContainerId())
	if err != nil {
		return nil, fmt.Errorf("could not find container %q: %w", req.GetContainerId(), err)
	}

	url, err := monitorStreamingURL(
		ctx,
		s.Runtime().RuntimeStreamWebsockets,
		s.getSandbox(ctx, c.Sandbox()).RuntimeHandler(),
		"exec",
		fmt.Sprintf("container %q", req.GetContainerId()),
		func() (string, error) {
			return s.Runtime().ServeExecContainer(
				ctx, c, req.GetCmd(), req.GetTty(), req.GetStdin(), req.GetStdout(), req.GetStderr(),
			)
		},
	)
	if err != nil {
		return nil, err
	}

	if url != "" {
		return &types.ExecResponse{Url: url}, nil
	}

	resp, err := s.getExec(req)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare exec endpoint: %w", err)
	}

	return resp, nil
}

// Exec endpoint for streaming.Runtime.
func (s *StreamService) Exec(
	ctx context.Context,
	containerID string,
	cmd []string,
	stdin io.Reader,
	stdout, stderr io.WriteCloser,
	tty bool,
	resizeChan <-chan remotecommand.TerminalSize,
) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	c, err := s.runtimeServer.GetContainerFromShortID(ctx, containerID)
	if err != nil {
		return status.Errorf(codes.NotFound, "could not find container %q: %v", containerID, err)
	}

	return execStatus(
		s.runtimeServer.ContainerServer.Runtime().
			ExecContainer(s.ctx, c, cmd, stdin, stdout, stderr, tty, resizeChan),
	)
}
