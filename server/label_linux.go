package server

import (
	"fmt"

	"github.com/opencontainers/selinux/go-selinux/label"
	"golang.org/x/sys/unix"
)

func securityLabel(path string, secLabel string, shared bool) error {
	if err := label.Relabel(path, secLabel, shared); err != nil && err != unix.ENOTSUP {
		return fmt.Errorf("relabel failed %s: %v", path, err)
	}
	return nil
}
