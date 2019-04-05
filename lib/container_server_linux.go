// +build linux

package lib

import (
	"github.com/cri-o/cri-o/lib/sandbox"
	selinux "github.com/opencontainers/selinux/go-selinux"
	"github.com/opencontainers/selinux/go-selinux/label"
)

func (c *ContainerServer) addSandboxPlatform(sb *sandbox.Sandbox) {
	// NewContext() always returns nil, so we can safely ignore the error
	ctx, _ := selinux.NewContext(sb.ProcessLabel())
	c.state.processLevels[ctx["level"]]++
}

func (c *ContainerServer) removeSandboxPlatform(sb *sandbox.Sandbox) {
	processLabel := sb.ProcessLabel()
	// NewContext() always returns nil, so we can safely ignore the error
	ctx, _ := selinux.NewContext(processLabel)
	level := ctx["level"]
	pl, ok := c.state.processLevels[level]
	if ok {
		c.state.processLevels[level] = pl - 1
		if c.state.processLevels[level] == 0 {
			label.ReleaseLabel(processLabel)
			delete(c.state.processLevels, level)
		}
	}
}
