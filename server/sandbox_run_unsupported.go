// +build !linux

package server

import (
	"context"
	"fmt"

	"github.com/cri-o/cri-o/server/cri/types"
)

func (s *Server) runPodSandbox(ctx context.Context, req *types.RunPodSandboxRequest) (*types.RunPodSandboxResponse, error) {
	return nil, fmt.Errorf("unsupported")
}
