package server

import (
	ctrfactory "github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/opencontainers/runtime-tools/generate"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var tmpfsMounts = map[string]string{
	"/tmp":     "mode=1777",
	"/var/run": "mode=0755",
	"/var/tmp": "mode=1777",
}

func getSecurityContext(containerConfig *types.ContainerConfig) *types.LinuxContainerSecurityContext {
	return &types.LinuxContainerSecurityContext{}
}

func (s *Server) setAppArmorProfile(ctx context.Context, ctr ctrfactory.Container, securityContext *types.LinuxContainerSecurityContext, specgen *generate.Generator) error {
	return nil
}

func (s *Server) setSecurityContextNamespaceOptions(ctx context.Context, ctr ctrfactory.Container, containerConfig *types.ContainerConfig, sb *sandbox.Sandbox) error {
	return nil
}
