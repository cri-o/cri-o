package container

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	devicecfg "github.com/cri-o/cri-o/internal/config/device"
	"tags.cncf.io/container-device-interface/pkg/cdi"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/utils"
	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/opencontainers/runc/libcontainer/devices"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *container) SpecAddDevices(configuredDevices, annotationDevices []devicecfg.Device, privilegedWithoutHostDevices, enableDeviceOwnershipFromSecurityContext bool) error {
	// First, clear the existing devices from the spec
	c.Spec().Config.Linux.Devices = []rspec.LinuxDevice{}

	// After that, add additional_devices from config
	for i := range configuredDevices {
		d := &configuredDevices[i]

		c.Spec().AddDevice(d.Device)
		c.Spec().AddLinuxResourcesDevice(d.Resource.Allow, d.Resource.Type, d.Resource.Major, d.Resource.Minor, d.Resource.Access)
	}

	// Next, verify and add the devices from annotations
	for i := range annotationDevices {
		d := &annotationDevices[i]

		c.Spec().AddDevice(d.Device)
		c.Spec().AddLinuxResourcesDevice(d.Resource.Allow, d.Resource.Type, d.Resource.Major, d.Resource.Minor, d.Resource.Access)
	}

	// Then, add host devices if privileged
	if err := c.specAddHostDevicesIfPrivileged(privilegedWithoutHostDevices); err != nil {
		return err
	}

	// Then, add container config devices
	if err := c.specAddContainerConfigDevices(enableDeviceOwnershipFromSecurityContext); err != nil {
		return err
	}

	// Finally, inject CDI devices
	return c.specInjectCDIDevices()
}

func (c *container) specAddHostDevicesIfPrivileged(privilegedWithoutHostDevices bool) error {
	if !c.Privileged() || privilegedWithoutHostDevices {
		return nil
	}
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
		c.Spec().AddDevice(rd)
	}
	c.Spec().Config.Linux.Resources.Devices = []rspec.LinuxDeviceCgroup{
		{
			Allow:  true,
			Access: "rwm",
		},
	}
	return nil
}

func (c *container) specAddContainerConfigDevices(enableDeviceOwnershipFromSecurityContext bool) error {
	sp := c.Spec().Config

	for _, device := range c.Config().Devices {
		// pin the device to avoid using `device` within the range scope as
		// wrong function literal
		device := device

		// If we are privileged, we have access to devices on the host.
		// If the requested container path already exists on the host, the container won't see the expected host path.
		// Therefore, we must error out if the container path already exists
		if c.Privileged() && device.ContainerPath != device.HostPath {
			// we expect this to not exist
			_, err := os.Stat(device.ContainerPath)
			if err == nil {
				return errors.New("privileged container was configured with a device container path that already exists on the host")
			}
			if !os.IsNotExist(err) {
				return fmt.Errorf("error checking if container path exists on host: %w", err)
			}
		}

		path, err := securejoin.SecureJoin("/", device.HostPath)
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
				UID:   getDeviceUserGroupID(c.Config().Linux.SecurityContext.RunAsUser, dev.Uid, enableDeviceOwnershipFromSecurityContext),
				GID:   getDeviceUserGroupID(c.Config().Linux.SecurityContext.RunAsGroup, dev.Gid, enableDeviceOwnershipFromSecurityContext),
			}
			c.Spec().AddDevice(rd)
			sp.Linux.Resources.Devices = append(sp.Linux.Resources.Devices, rspec.LinuxDeviceCgroup{
				Allow:  true,
				Type:   string(dev.Type),
				Major:  &dev.Major,
				Minor:  &dev.Minor,
				Access: string(dev.Permissions),
			})
			continue
		}
		// if the device is not a device node
		// try to see if it's a directory holding many devices
		if errors.Is(err, devices.ErrNotADevice) {
			// check if it is a directory
			if e := utils.IsDirectory(path); e == nil {
				// mount the internal devices recursively
				// nolint: errcheck
				filepath.Walk(path, func(dpath string, f os.FileInfo, e error) error {
					// filepath.Walk failed, skip
					if e != nil {
						return nil
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
					c.Spec().AddDevice(rd)
					sp.Linux.Resources.Devices = append(sp.Linux.Resources.Devices, rspec.LinuxDeviceCgroup{
						Allow:  true,
						Type:   string(childDevice.Type),
						Major:  &childDevice.Major,
						Minor:  &childDevice.Minor,
						Access: string(childDevice.Permissions),
					})

					return nil
				})
			}
		}
	}
	return nil
}

func (c *container) specInjectCDIDevices() error {
	var (
		cdiDevices = c.Config().CDIDevices
		fromCRI    = map[string]struct{}{}
		requested  = make([]string, 0, len(cdiDevices))
		annotated  []string
		err        error
	)

	// Take CDI devices from the dedicated CDIDevices CRI field.
	for _, dev := range cdiDevices {
		requested = append(requested, dev.Name)
		fromCRI[dev.Name] = struct{}{}
	}

	// Extract CDI devices from annotations which is still supported as a means
	// of injecting CDI devices to give people time to update their DRA drivers.
	// TODO(klihub): Change the log message to a warning once annotations are
	// deprecated, and to an error once support is removed altogether.
	_, annotated, err = cdi.ParseAnnotations(c.Config().GetAnnotations())
	if err != nil {
		return fmt.Errorf("failed to parse CDI device annotations: %w", err)
	}

	// Allow injecting the same device using both a dedicated field and an
	// annotation. This helps the transition from annotations to dedicated
	// CDI fields. DRA drivers can be updated to first inject devices using
	// both. This allows updated drivers to be used in clusters that still
	// talk old, pre-CDIDevices CRI. Then once annotations are deprecated
	// drivers can be updated to stop using annotations. This also mirrors
	// the behavior implemented in containerd.
	if len(annotated) > 0 {
		for _, name := range annotated {
			if _, ok := fromCRI[name]; ok {
				// TODO(klihub): change to a warning once annotations are deprecated
				log.Infof(context.TODO(),
					"Skipping duplicate annotated CDI device %s", name)
				continue
			}
			requested = append(requested, name)
		}
		// TODO(klihub): change to a warning once annotations are deprecated
		log.Infof(context.TODO(),
			"Passing CDI devices as annotations will be deprecated soon "+
				"please use the CDIDevices CRI field instead")
	}

	if len(requested) == 0 {
		return nil
	}

	registry := cdi.GetRegistry()
	if err := registry.Refresh(); err != nil {
		// We don't consider registry refresh failure a fatal error.
		// For instance, a dynamically generated invalid CDI Spec file for
		// any particular vendor shouldn't prevent injection of devices of
		// different vendors. CDI itself knows better and it will fail the
		// injection if necessary.

		log.Warnf(context.TODO(), "CDI registry has errors: %v", err)
	}

	if _, err := registry.InjectDevices(c.Spec().Config, requested...); err != nil {
		return fmt.Errorf("CDI device injection failed: %w", err)
	}

	// One crucial thing to keep in mind is that CDI device injection
	// might add OCI Spec environment variables, hooks, and mounts as
	// well. Therefore it is important that none of the corresponding
	// OCI Spec fields are reset up in the call stack once we return.
	return nil
}

// getDeviceUserGroupID() is used to find the right uid/gid
// value for the device node created in the container namespace.
// The runtime executes mknod() and chmod()s the created
// device with the values returned here.
//
// TODO(mythi): In case of user namespaces, the runtime simply bind
// mounts the devices from the host. Additional logic is needed
// to check that the runtimes effective UID/GID on the host has the
// permissions to access the device node and/or the right user namespace
// mappings are created.
//
// CRI-O has an experimental support for setting user namespace mappings
// via annotations when pod's securitycontext runs as root/uid=0. When
// enabled, the logic below does not change the behavior for existing
// mappings unless container's securitycontext user/group overrides what
// is set for the pod.
//
// Ref: https://github.com/kubernetes/kubernetes/issues/92211
func getDeviceUserGroupID(runAsVal *types.Int64Value, hostVal uint32, enableDeviceOwnershipFromSecurityContext bool) *uint32 {
	if runAsVal != nil {
		id := uint32(runAsVal.Value)
		if id > 0 && enableDeviceOwnershipFromSecurityContext {
			return &id
		}
	}
	return &hostVal
}
