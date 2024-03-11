//go:build test
// +build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package container

import (
	"context"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/storage"
	sconfig "github.com/cri-o/cri-o/pkg/config"
)

func GetContainerInfo(ctr Container) *container {
	return ctr.(*container)
}

func (ctr *container) NewMountInfo() {
	ctr.mountInfo = newMountInfo()
}

func (ctr *container) ClearMountInfo() {
	clearMountInfo(ctr)
}

func (ctr *container) SetupHostNetworkMounts(hostNetwork bool, options []string) {
	ctr.setupHostNetworkMounts(hostNetwork, options)
	specAddMounts(ctr)
}

func (ctr *container) SetupPrivilegedMounts() {
	ctr.setupPrivilegedMounts()
	specAddMounts(ctr)
}

func (ctr *container) SetupReadOnlyMounts(readOnly bool) {
	ctr.setupReadOnlyMounts(readOnly)
	specAddMounts(ctr)
}

func (ctr *container) SetupShmMounts(shmPath string) {
	ctr.setupShmMounts(shmPath)
	specAddMounts(ctr)
}

func (ctr *container) SetupHostPropMounts(sb *sandbox.Sandbox, mountLabel string, options []string) error {
	if err := ctr.setupHostPropMounts(sb, mountLabel, options); err != nil {
		return err
	}
	specAddMounts(ctr)
	return nil
}

func (ctr *container) SetOCIBindMountsPrivileged() {
	ctr.setOCIBindMountsPrivileged()
	specAddMounts(ctr)
}

func (ctr *container) AddImageVolumes(ctx context.Context, rootfs string, serverConfig *sconfig.Config, containerInfo *storage.ContainerInfo) error {
	if err := ctr.addImageVolumes(ctx, rootfs, serverConfig, containerInfo); err != nil {
		return err
	}
	specAddMounts(ctr)
	return nil
}

func (ctr *container) SetupSecretMounts(defaultMountsFile string, containerInfo storage.ContainerInfo, mountPoint string) {
	_ = ctr.setupSecretMounts(defaultMountsFile, containerInfo, mountPoint)
	specAddMounts(ctr)
}

func (ctr *container) AddOCIBindMounts(ctx context.Context, mountLabel string, serverConfig *sconfig.Config, maybeRelabel, idMapSupport, cgroup2RW bool) error {
	if _, err := ctr.addOCIBindMounts(ctx, mountLabel, serverConfig, maybeRelabel, idMapSupport, cgroup2RW); err != nil {
		return err
	}
	specAddMounts(ctr)
	return nil
}

func (ctr *container) SetupSystemdMounts(containerInfo storage.ContainerInfo) error {
	if err := ctr.setupSystemdMounts(containerInfo); err != nil {
		return err
	}
	specAddMounts(ctr)
	return nil
}

func (ctr *container) SetupTimeZone(tz, containerRunDir, containerID, mountPoint, mountLabel string, options []string) error {
	if err := ctr.setupTimeZone(tz, containerRunDir, containerID, mountPoint, mountLabel, options); err != nil {
		return err
	}
	specAddMounts(ctr)
	return nil
}
