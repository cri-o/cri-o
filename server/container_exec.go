package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/cri-streaming/pkg/streaming/remotecommand"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
)

// execLivenessTimeout bounds the liveness check made before an exec, so a
// container runtime which does not answer cannot hold the request open.
const execLivenessTimeout = 10 * time.Second

// ensureContainerExecAllowed asks the runtime whether the container can take a
// new process before serving an exec.
func ensureContainerExecAllowed(ctx context.Context, rt *oci.Runtime, c *oci.Container) error {
	aliveCtx, cancel := context.WithTimeout(ctx, execLivenessTimeout)
	defer cancel()

	alive, err := rt.IsContainerAlive(aliveCtx, c)
	if err != nil {
		code := codes.Unavailable

		switch {
		case errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded:
			code = codes.DeadlineExceeded
		case errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled:
			code = codes.Canceled
		}

		return status.Errorf(code, "unable to determine whether container is running: %v", err)
	}

	if !alive {
		return status.Errorf(codes.NotFound, "container is not created or running")
	}

	return nil
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

	// Ask the runtime instead of looking at the init pid CRI-O cached when the
	// container was created. A VM runtime uses the VM's pid as the container
	// pid, and that pid changes when the VM is restored from a checkpoint, so
	// the cached one refuses execs for containers which are running fine.
	if err := ensureContainerExecAllowed(
		ctx, s.runtimeServer.Runtime(), c,
	); err != nil {
		return err
	}

	return s.runtimeServer.ContainerServer.Runtime().
		ExecContainer(s.ctx, c, cmd, stdin, stdout, stderr, tty, resizeChan)
}
