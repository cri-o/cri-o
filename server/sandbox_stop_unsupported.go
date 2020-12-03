// +build !linux

package server

import (
	"context"
	"fmt"

	"github.com/cri-o/cri-o/server/cri/types"
)

func (s *Server) stopPodSandbox(ctx context.Context, req *types.StopPodSandboxRequest) error {
	return fmt.Errorf("unsupported")
}
