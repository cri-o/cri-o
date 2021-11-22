//go:build !linux
// +build !linux

package server

import (
	"context"
	"fmt"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *Server) stopPodSandbox(ctx context.Context, req *types.StopPodSandboxRequest) error {
	return fmt.Errorf("unsupported")
}
