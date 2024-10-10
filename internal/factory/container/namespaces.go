package container

import (
	"github.com/cri-o/cri-o/internal/config/nsmgr"
)

func (c *container) PidNamespace() nsmgr.Namespace {
	return c.pidns
}
