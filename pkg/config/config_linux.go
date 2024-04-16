package config

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/containers/storage/pkg/parsers/kernel"
	selinux "github.com/opencontainers/selinux/go-selinux"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// Defaults if none are specified
const (
	defaultRuntime       = "runc"
	DefaultRuntimeType   = "oci"
	DefaultRuntimeRoot   = "/run/runc"
	defaultMonitorCgroup = "system.slice"
	// ImageVolumesBind option is for using bind mounted volumes
	ImageVolumesBind ImageVolumesType = "bind"
	// DefaultPauseImage is default pause image
	DefaultPauseImage string = "registry.k8s.io/pause:3.9"
)

var (
	kernelRROSupportOnce  sync.Once
	kernelRROSupportError error
)

func selinuxEnabled() bool {
	return selinux.GetEnabled()
}

func (c *RuntimeConfig) ValidatePinnsPath(executable string) error {
	var err error
	c.PinnsPath, err = validateExecutablePath(executable, c.PinnsPath)

	return err
}

// checkKernelRROMountSupport checks the kernel support for the Recursive Read-only (RRO) mounts.
func checkKernelRROMountSupport() error {
	kernelRROSupportOnce.Do(func() {
		kernelRROSupportError = (func() error {
			// Check the current kernel version for RRO mounts support...
			err := validateKernelRROVersion()
			if err != nil {
				versionErr := err
				// ... and if the kernel version does not match the minimum required version,
				// then verify whether the kernel supports RRO mounts regardless, as often
				// Linux distributions provide heavily patched kernel releases, and the
				// current kernel might include backported support.
				err = validateKernelRROMount()
				if err != nil {
					err = fmt.Errorf("%w: %w", versionErr, err)
				}
			}
			return err
		})()
	})

	return kernelRROSupportError
}

// validateKernelRROVersion checks whether the current kernel version matches the release 5.12 or newer,
// which is the minimum required kernel version that supports Recursive Read-only (RRO) mounts.
func validateKernelRROVersion() error {
	kv, err := kernel.GetKernelVersion()
	if err != nil {
		return fmt.Errorf("unable to retrieve kernel version: %w", err)
	}

	result := kernel.CompareKernelVersion(*kv,
		kernel.VersionInfo{
			Kernel: 5,
			Major:  12,
			Minor:  0,
		},
	)
	if result < 0 {
		return fmt.Errorf("kernel version %q does not support recursive read-only mounts", kv)
	}

	return nil
}

// validateKernelRROMount checks whether the current kernel can support Recursive Read-only mounts.
// It uses a test mount of tmpfs against which an attempt will be made to set the required attributes.
// If there is no failure in doing so, then the kernel has the required support.
func validateKernelRROMount() error {
	path, err := os.MkdirTemp("", "crio-rro-*")
	if err != nil {
		return fmt.Errorf("unable to create directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(path); err != nil {
			logrus.Errorf("Unable to remove directory: %v", err)
		}
	}()

	for {
		err = unix.Mount("", path, "tmpfs", 0, "")
		if !errors.Is(err, unix.EINTR) {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("unable to mount directory %q using tmpfs: %w", path, err)
	}
	defer func() {
		var unmountErr error
		for {
			unmountErr = unix.Unmount(path, 0)
			if !errors.Is(unmountErr, unix.EINTR) {
				break
			}
		}
		if unmountErr != nil {
			logrus.Errorf("Unable to unmount directory %q: %v", path, unmountErr)
		}
	}()

	for {
		err = unix.MountSetattr(-1, path, unix.AT_RECURSIVE,
			&unix.MountAttr{
				Attr_set: unix.MOUNT_ATTR_RDONLY,
			},
		)
		if !errors.Is(err, unix.EINTR) {
			break
		}
	}
	if err != nil {
		if !errors.Is(err, unix.ENOSYS) {
			return fmt.Errorf("unable to set mount attribute for directory %q: %w", path, err)
		}
		return fmt.Errorf("unable to set recursive read-only mount attribute: %w", err)
	}

	return nil
}
