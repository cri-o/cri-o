//go:build !linux
// +build !linux

package lib

import (
	"github.com/cri-o/cri-o/internal/lib/sandbox"
)

func (c *ContainerServer) addSandboxPlatform(sb *sandbox.Sandbox) error {
	return nil
}

func (c *ContainerServer) removeSandboxPlatform(sb *sandbox.Sandbox) error {
	return nil
}
