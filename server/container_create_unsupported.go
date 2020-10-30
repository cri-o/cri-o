// +build !linux

package server

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/sandbox"
	"github.com/opencontainers/runtime-tools/generate"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func findCgroupMountpoint(name string) error {
	return fmt.Errorf("no cgroups on this platform")
}

func addDevicesPlatform(sb *sandbox.Sandbox, containerConfig *pb.ContainerConfig, privilegedWithoutHostDevices bool, specgen *generate.Generator) error {
	return nil
}

func (s *Server) createSandboxContainer(ctx context.Context, ctr container.Container, sb *sandbox.Sandbox) (*oci.Container, error) {
	return nil, fmt.Errorf("not implemented yet")
}
