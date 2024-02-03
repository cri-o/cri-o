package server

import (
	ctrfactory "github.com/cri-o/cri-o/internal/factory/container"
	"github.com/opencontainers/runtime-tools/generate"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func getSecurityContext(containerConfig *types.ContainerConfig) *types.LinuxContainerSecurityContext {
	return &types.LinuxContainerSecurityContext{}
}

func (s *Server) setAppArmorProfile(ctx context.Context, ctr ctrfactory.Container, securityContext *types.LinuxContainerSecurityContext, specgen *generate.Generator) error {
	return nil
}
