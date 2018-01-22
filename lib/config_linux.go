// +build linux

package lib

import selinux "github.com/opencontainers/selinux/go-selinux"

func selinuxEnabled() bool {
	return selinux.GetEnabled()
}
