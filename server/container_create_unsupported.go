// +build !linux

package server

import (
	"fmt"

	"github.com/kubernetes-incubator/cri-o/lib/sandbox"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/opencontainers/runtime-tools/generate"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

func findCgroupMountpoint(name string) error {
	return fmt.Errorf("no cgroups on this platform")
}

func addDevicesPlatform(sb *sandbox.Sandbox, containerConfig *pb.ContainerConfig, specgen *generate.Generator) error {
	return nil
}

func (s *Server) createSandboxContainer(ctx context.Context, containerID string, containerName string, sb *sandbox.Sandbox, sandboxConfig *pb.PodSandboxConfig, containerConfig *pb.ContainerConfig) (*oci.Container, error) {
	return nil, fmt.Errorf("not implemented yet")
}
