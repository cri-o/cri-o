// +build linux

package lib

import (
	"github.com/kubernetes-sigs/cri-o/lib/sandbox"
	selinux "github.com/opencontainers/selinux/go-selinux"
	"github.com/opencontainers/selinux/go-selinux/label"
)

func (c *ContainerServer) addSandboxPlatform(sb *sandbox.Sandbox) error {
	selinuxCtx, err := selinux.NewContext(sb.ProcessLabel())
	if err != nil {
		return err
	}
	c.state.processLevels[selinuxCtx["level"]]++
	return nil
}

func (c *ContainerServer) removeSandboxPlatform(sb *sandbox.Sandbox) error {
	processLabel := sb.ProcessLabel()
	selinuxCtx, err := selinux.NewContext(processLabel)
	if err != nil {
		return err
	}
	level := selinuxCtx["level"]
	pl, ok := c.state.processLevels[level]
	if ok {
		c.state.processLevels[level] = pl - 1
		if c.state.processLevels[level] == 0 {
			label.ReleaseLabel(processLabel)
			delete(c.state.processLevels, level)
		}
	}
	return nil
}
