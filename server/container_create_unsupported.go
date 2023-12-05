//go:build !linux
// +build !linux

package server

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	"golang.org/x/net/context"
)

func (s *Server) createSandboxContainer(ctx context.Context, ctr container.Container, sb *sandbox.Sandbox) (*oci.Container, error) {
	return nil, fmt.Errorf("not implemented yet")
}
