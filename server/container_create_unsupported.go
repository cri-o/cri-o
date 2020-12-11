// +build !linux

package server

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/container"
	"github.com/cri-o/cri-o/pkg/sandbox"
	"golang.org/x/net/context"
)

func findCgroupMountpoint(name string) error {
	return fmt.Errorf("no cgroups on this platform")
}

func (s *Server) createSandboxContainer(ctx context.Context, ctr container.Container, sb *sandbox.Sandbox) (*oci.Container, error) {
	return nil, fmt.Errorf("not implemented yet")
}
