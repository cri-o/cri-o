package server

import (
	"context"
)

// CheckIfPodCheckpointOCIImage exports checkIfPodCheckpointOCIImage for testing.
func (s *Server) CheckIfPodCheckpointOCIImage(ctx context.Context, input string) (*PodCheckpointInfo, error) {
	return s.checkIfPodCheckpointOCIImage(ctx, input)
}
