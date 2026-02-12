package server

import (
	"context"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// CheckIfPodCheckpointOCIImage exports checkIfPodCheckpointOCIImage for testing.
func (s *Server) CheckIfPodCheckpointOCIImage(ctx context.Context, input string) (*PodCheckpointInfo, error) {
	return s.checkIfPodCheckpointOCIImage(ctx, input)
}

// RestorePodContainers exports restorePodContainers for testing.
func (s *Server) RestorePodContainers(
	ctx context.Context,
	newPodID string,
	mountPoint string,
	podCheckpoint *PodCheckpointInfo,
	req *types.RestorePodRequest,
	checkpointedPodOptions *metadata.CheckpointedPodOptions,
) (*types.RestorePodResponse, error) {
	return s.restorePodContainers(ctx, newPodID, mountPoint, podCheckpoint, req, checkpointedPodOptions)
}
