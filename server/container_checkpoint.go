package server

import (
	"fmt"

	"github.com/containers/podman/v4/libpod"
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/log"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// CheckpointContainer checkpoints a container
func (s *Server) CheckpointContainer(ctx context.Context, req *types.CheckpointContainerRequest) (*types.CheckpointContainerResponse, error) {
	if !s.config.RuntimeConfig.CheckpointRestore() {
		return nil, fmt.Errorf("checkpoint/restore support not available")
	}

	_, err := s.GetContainerFromShortID(ctx, req.ContainerId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerId, err)
	}
	log.Infof(ctx, "Checkpointing container: %s", req.ContainerId)
	opts := &lib.ContainerCheckpointRestoreOptions{
		Container: req.ContainerId,
		ContainerCheckpointOptions: libpod.ContainerCheckpointOptions{
			TargetFile: req.Location,
			// For the forensic container checkpointing use case we
			// keep the container running after checkpointing it.
			KeepRunning: true,
		},
	}

	_, err = s.ContainerServer.ContainerCheckpoint(ctx, opts)
	if err != nil {
		return nil, err
	}

	log.Infof(ctx, "Checkpointed container: %s", req.ContainerId)

	return &types.CheckpointContainerResponse{}, nil
}
