package server

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
)

// ExecSync runs a command in a container synchronously.
func (s *Server) ExecSync(
	ctx context.Context,
	req *types.ExecSyncRequest,
) (*types.ExecSyncResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	c, err := s.GetContainerFromShortID(ctx, req.GetContainerId())
	if err != nil {
		return nil, status.Errorf(
			codes.NotFound,
			"could not find container %q: %v",
			req.GetContainerId(),
			err,
		)
	}

	// Ask the runtime instead of looking at the init pid CRI-O cached when the
	// container was created. A VM runtime uses the VM's pid as the container
	// pid, and that pid changes when the VM is restored from a checkpoint, so
	// the cached one refuses execs for containers which are running fine.
	aliveCtx, cancel := context.WithTimeout(ctx, execLivenessTimeout)
	defer cancel()

	alive, err := s.ContainerServer.Runtime().IsContainerAlive(aliveCtx, c)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "container is not created or running: %v", err)
	}

	if !alive {
		return nil, status.Errorf(codes.NotFound, "container is not created or running")
	}

	cmd := req.GetCmd()
	if cmd == nil {
		return nil, errors.New("exec command cannot be empty")
	}

	return s.ContainerServer.Runtime().ExecSyncContainer(ctx, c, cmd, req.GetTimeout())
}
