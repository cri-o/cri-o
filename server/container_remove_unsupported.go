//go:build !linux
// +build !linux

package server

import (
	"context"

	"github.com/cri-o/cri-o/internal/oci"
)

func (s *Server) removeSeccompNotifier(ctx context.Context, c *oci.Container) {
}
