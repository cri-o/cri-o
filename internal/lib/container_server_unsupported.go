//go:build !linux
// +build !linux

package lib

import (
	"github.com/cri-o/cri-o/internal/lib/sandbox"
)

func (c *ContainerServer) addSandboxPlatform(sb *sandbox.Sandbox) {
	// nothing' doin'
}

func (c *ContainerServer) removeSandboxPlatform(sb *sandbox.Sandbox) {
	// nothing' doin'
}
