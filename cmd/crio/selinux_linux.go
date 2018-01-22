// +build linux

package main

import selinux "github.com/opencontainers/selinux/go-selinux"

func disableSELinux() {
	selinux.SetDisabled()
}
