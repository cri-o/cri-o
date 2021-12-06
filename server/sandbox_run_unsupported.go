//go:build !linux
// +build !linux

package server

import (
	"context"
	"fmt"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *Server) runPodSandbox(ctx context.Context, req *types.RunPodSandboxRequest) (*types.RunPodSandboxResponse, error) {
	return nil, fmt.Errorf("unsupported")
}
