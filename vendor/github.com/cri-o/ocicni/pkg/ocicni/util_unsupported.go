//go:build !linux && !freebsd
// +build !linux,!freebsd

package ocicni

import (
	"context"
	"errors"
	"net"
)

type nsManager struct {
}

var errUnsupportedPlatform = errors.New("unsupported platform")

func (nsm *nsManager) init() error {
	return nil
}

func getContainerDetails(_ context.Context, nsm *nsManager, netnsPath, interfaceName, addrType string) (*net.IPNet, *net.HardwareAddr, error) {
	return nil, nil, errUnsupportedPlatform
}

func bringUpLoopback(netns string) error {
	return errUnsupportedPlatform
}

func checkLoopback(netns string) error {
	return errUnsupportedPlatform

}
