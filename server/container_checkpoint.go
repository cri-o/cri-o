package server

import (
	"context"
	"errors"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/log"
)

// CheckpointContainer checkpoints a container.
func (s *Server) CheckpointContainer(ctx context.Context, req *types.CheckpointContainerRequest) (*types.CheckpointContainerResponse, error) {
	if !s.config.CheckpointRestore() {
		return nil, errors.New("checkpoint/restore support not available")
	}

	_, err := s.GetContainerFromShortID(ctx, req.GetContainerId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.GetContainerId(), err)
	}

	log.Infof(ctx, "Checkpointing container: %s", req.GetContainerId())
	config := &metadata.ContainerConfig{
		ID: req.GetContainerId(),
	}
	opts := &lib.ContainerCheckpointOptions{
		TargetFile: req.GetLocation(),
		// For the forensic container checkpointing use case we
		// keep the container running after checkpointing it.
		KeepRunning: true,
	}

	_, err = s.ContainerCheckpoint(ctx, config, opts)
	if err != nil {
		return nil, err
	}

	log.Infof(ctx, "Checkpointed container: %s", req.GetContainerId())

	return &types.CheckpointContainerResponse{}, nil
}
