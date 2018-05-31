// +build linux

package server

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/containers/storage/pkg/idtools"
	"github.com/kubernetes-incubator/cri-o/lib/sandbox"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/devices"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

func findCgroupMountpoint(name string) error {
	// Set up pids limit if pids cgroup is mounted
	_, err := cgroups.FindCgroupMountpoint(name)
	return err
}

func addDevicesPlatform(sb *sandbox.Sandbox, containerConfig *pb.ContainerConfig, specgen *generate.Generator) error {
	sp := specgen.Spec()
	if containerConfig.GetLinux().GetSecurityContext().GetPrivileged() {
		hostDevices, err := devices.HostDevices()
		if err != nil {
			return err
		}
		for _, hostDevice := range hostDevices {
			rd := rspec.LinuxDevice{
				Path:  hostDevice.Path,
				Type:  string(hostDevice.Type),
				Major: hostDevice.Major,
				Minor: hostDevice.Minor,
				UID:   &hostDevice.Uid,
				GID:   &hostDevice.Gid,
			}
			if hostDevice.Major == 0 && hostDevice.Minor == 0 {
				// Invalid device, most likely a symbolic link, skip it.
				continue
			}
			specgen.AddDevice(rd)
		}
		sp.Linux.Resources.Devices = []rspec.LinuxDeviceCgroup{
			{
				Allow:  true,
				Access: "rwm",
			},
		}
		return nil
	}
	for _, device := range containerConfig.GetDevices() {
		path, err := resolveSymbolicLink(device.HostPath, "/")
		if err != nil {
			return err
		}
		dev, err := devices.DeviceFromPath(path, device.Permissions)
		// if there was no error, return the device
		if err == nil {
			rd := rspec.LinuxDevice{
				Path:  device.ContainerPath,
				Type:  string(dev.Type),
				Major: dev.Major,
				Minor: dev.Minor,
				UID:   &dev.Uid,
				GID:   &dev.Gid,
			}
			specgen.AddDevice(rd)
			sp.Linux.Resources.Devices = append(sp.Linux.Resources.Devices, rspec.LinuxDeviceCgroup{
				Allow:  true,
				Type:   string(dev.Type),
				Major:  &dev.Major,
				Minor:  &dev.Minor,
				Access: dev.Permissions,
			})
			continue
		}
		// if the device is not a device node
		// try to see if it's a directory holding many devices
		if err == devices.ErrNotADevice {

			// check if it is a directory
			if src, e := os.Stat(path); e == nil && src.IsDir() {

				// mount the internal devices recursively
				filepath.Walk(path, func(dpath string, f os.FileInfo, e error) error {
					if e != nil {
						logrus.Debugf("addDevice walk: %v", e)
					}
					childDevice, e := devices.DeviceFromPath(dpath, device.Permissions)
					if e != nil {
						// ignore the device
						return nil
					}
					cPath := strings.Replace(dpath, path, device.ContainerPath, 1)
					rd := rspec.LinuxDevice{
						Path:  cPath,
						Type:  string(childDevice.Type),
						Major: childDevice.Major,
						Minor: childDevice.Minor,
						UID:   &childDevice.Uid,
						GID:   &childDevice.Gid,
					}
					specgen.AddDevice(rd)
					sp.Linux.Resources.Devices = append(sp.Linux.Resources.Devices, rspec.LinuxDeviceCgroup{
						Allow:  true,
						Type:   string(childDevice.Type),
						Major:  &childDevice.Major,
						Minor:  &childDevice.Minor,
						Access: childDevice.Permissions,
					})

					return nil
				})
			}
		}
	}
	return nil
}

// createContainerPlatform performs platform dependent intermediate steps before calling the container's oci.Runtime().CreateContainer()
func (s *Server) createContainerPlatform(container *oci.Container, infraContainer *oci.Container, cgroupParent string) error {
	intermediateMountPoint := container.IntermediateMountPoint()

	if intermediateMountPoint == "" {
		return s.Runtime().CreateContainer(container, cgroupParent)
	}

	errc := make(chan error)
	go func() {
		// We create a new mount namespace before running the container as the rootfs of the
		// container is accessible only to the root user.  We use the intermediate mount
		// namespace to bind mount the root to a directory that is accessible to the user which
		// maps to root inside of the container/
		// We carefully unlock the OS thread only if no errors happened.  The thread might have failed
		// to restore the original mount namespace, and unlocking it will let it keep running
		// in a different context than the other threads.  A thread that is still locked when the
		// goroutine terminates is automatically destroyed.
		var err error
		runtime.LockOSThread()
		defer func() {
			if err == nil {
				runtime.UnlockOSThread()
			}
			errc <- err
		}()

		fd, err := os.Open(fmt.Sprintf("/proc/%d/task/%d/ns/mnt", os.Getpid(), unix.Gettid()))
		if err != nil {
			return
		}
		defer fd.Close()

		// create a new mountns on the current thread
		if err = unix.Unshare(unix.CLONE_NEWNS); err != nil {
			return
		}
		defer unix.Setns(int(fd.Fd()), unix.CLONE_NEWNS)

		// don't spread our mounts around
		err = unix.Mount("/", "/", "none", unix.MS_REC|unix.MS_SLAVE, "")
		if err != nil {
			return
		}

		rootUID, rootGID, err := idtools.GetRootUIDGID(container.IDMappings().UIDs(), container.IDMappings().GIDs())
		if err != nil {
			return
		}

		err = os.Chown(intermediateMountPoint, rootUID, rootGID)
		if err != nil {
			return
		}

		mountPoint := container.MountPoint()
		err = os.Chown(mountPoint, rootUID, rootGID)
		if err != nil {
			return
		}

		rootPath := filepath.Join(intermediateMountPoint, "root")
		err = idtools.MkdirAllAs(rootPath, 0700, rootUID, rootGID)
		if err != nil {
			return
		}

		err = unix.Mount(mountPoint, rootPath, "none", unix.MS_BIND, "")
		if err != nil {
			return
		}

		if infraContainer != nil {
			infraRunDir := filepath.Join(intermediateMountPoint, "infra-rundir")
			err = idtools.MkdirAllAs(infraRunDir, 0700, rootUID, rootGID)
			if err != nil {
				return
			}

			err = unix.Mount(infraContainer.BundlePath(), infraRunDir, "none", unix.MS_BIND, "")
			if err != nil {
				return
			}
			err = os.Chown(infraRunDir, rootUID, rootGID)
			if err != nil {
				return
			}
		}

		runDirPath := filepath.Join(intermediateMountPoint, "rundir")
		err = os.MkdirAll(runDirPath, 0700)
		if err != nil {
			return
		}

		err = unix.Mount(container.BundlePath(), runDirPath, "none", unix.MS_BIND, "suid")
		if err != nil {
			return
		}
		err = os.Chown(runDirPath, rootUID, rootGID)
		if err != nil {
			return
		}

		err = s.Runtime().CreateContainer(container, cgroupParent)
	}()

	err := <-errc
	return err
}
