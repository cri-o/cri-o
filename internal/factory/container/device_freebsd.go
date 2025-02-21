package container

import (
	devicecfg "github.com/cri-o/cri-o/internal/config/device"
)

func (c *container) SpecAddDevices(configuredDevices, annotationDevices []devicecfg.Device, privilegedWithoutHostDevices, enableDeviceOwnershipFromSecurityContext bool) error {
	return nil
}
