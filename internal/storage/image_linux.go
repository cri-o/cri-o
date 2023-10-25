package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/podman/v4/pkg/rootless"
	"github.com/cri-o/cri-o/internal/dbusmgr"
	"github.com/cri-o/cri-o/utils"
)

// moveSelfToCgroup moves the current process to a new transient cgroup.
func moveSelfToCgroup(cgroup string) error {
	slice := "system.slice"
	if rootless.IsRootless() {
		slice = "user.slice"
	}

	if cgroup != "" {
		if !strings.Contains(cgroup, ".slice") {
			return fmt.Errorf("invalid systemd cgroup %q", cgroup)
		}
		slice = filepath.Base(cgroup)
	}

	unitName := fmt.Sprintf("crio-pull-image-%d.scope", os.Getpid())

	return utils.RunUnderSystemdScope(dbusmgr.NewDbusConnManager(rootless.IsRootless()), os.Getpid(), slice, unitName)
}
