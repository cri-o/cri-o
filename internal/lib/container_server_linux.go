package lib

import (
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	selinux "github.com/opencontainers/selinux/go-selinux"
	"github.com/opencontainers/selinux/go-selinux/label"
)

func (c *ContainerServer) addSandboxPlatform(sb *sandbox.Sandbox) error {
	context, err := selinux.NewContext(sb.ProcessLabel())
	if err != nil {
		return err
	}
	c.state.processLevels[context["level"]]++
	return nil
}

func (c *ContainerServer) removeSandboxPlatform(sb *sandbox.Sandbox) error {
	processLabel := sb.ProcessLabel()
	context, err := selinux.NewContext(processLabel)
	if err != nil {
		return err
	}
	level := context["level"]
	pl, ok := c.state.processLevels[level]
	if ok {
		c.state.processLevels[level] = pl - 1
		if c.state.processLevels[level] == 0 {
			defer delete(c.state.processLevels, level)
			if err := label.ReleaseLabel(processLabel); err != nil {
				return err
			}
		}
	}
	return nil
}
