package server

import (
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func getSecurityContext(containerConfig *types.ContainerConfig) *types.LinuxContainerSecurityContext {
	return &types.LinuxContainerSecurityContext{}
}
