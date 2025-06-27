package container

import (
	"fmt"
	"runtime"

	devicecfg "github.com/cri-o/cri-o/internal/config/device"
)

func (c *container) SpecAddDevices(configuredDevices, annotationDevices []devicecfg.Device, privilegedWithoutHostDevices, enableDeviceOwnershipFromSecurityContext bool) error {
	return nil
}

func (c *container) SpecInjectCDIDevices() error {
	if len(c.Config().CDIDevices) > 0 {
		return fmt.Errorf("(*container).SpecInjectCDIDevices not supported on %s", runtime.GOOS)
	}
	return nil
}
