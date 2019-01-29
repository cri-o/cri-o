// +build !linux

package lib

import (
	"github.com/kubernetes-sigs/cri-o/lib/sandbox"
)

func (c *ContainerServer) addSandboxPlatform(sb *sandbox.Sandbox) error {
	return nil
}

func (c *ContainerServer) removeSandboxPlatform(sb *sandbox.Sandbox) error {
	return nil
}
