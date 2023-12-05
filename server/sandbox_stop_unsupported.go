//go:build !linux
// +build !linux

package server

import (
	"context"
	"fmt"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
)

func (s *Server) stopPodSandbox(ctx context.Context, sb *sandbox.Sandbox) error {
	return fmt.Errorf("unsupported")
}
