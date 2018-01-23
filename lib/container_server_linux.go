// +build linux

package lib

import (
	"github.com/kubernetes-incubator/cri-o/lib/sandbox"
	selinux "github.com/opencontainers/selinux/go-selinux"
	"github.com/opencontainers/selinux/go-selinux/label"
)

func (c *ContainerServer) addSandboxPlatform(sb *sandbox.Sandbox) {
	c.state.processLevels[selinux.NewContext(sb.ProcessLabel())["level"]]++
}

func (c *ContainerServer) removeSandboxPlatform(sb *sandbox.Sandbox) {
	processLabel := sb.ProcessLabel()
	level := selinux.NewContext(processLabel)["level"]
	pl, ok := c.state.processLevels[level]
	if ok {
		c.state.processLevels[level] = pl - 1
		if c.state.processLevels[level] == 0 {
			label.ReleaseLabel(processLabel)
			delete(c.state.processLevels, level)
		}
	}
}
