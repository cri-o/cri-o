// +build !linux

package server

import (
	"fmt"

	"github.com/kubernetes-incubator/cri-o/lib/sandbox"
	"github.com/opencontainers/runtime-tools/generate"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

func findCgroupMountpoint(name string) error {
	return fmt.Errorf("no cgroups on this platform")
}

func addDevicesPlatform(sb *sandbox.Sandbox, containerConfig *pb.ContainerConfig, specgen *generate.Generator) error {
	return nil
}
