//go:build !linux && !freebsd

package ocicni

import (
	"context"
	"errors"
	"net"
)

type nsManager struct{}

var errUnsupportedPlatform = errors.New("unsupported platform")

func (nsm *nsManager) init() error {
	return nil
}

func getContainerDetails(_ context.Context, _ *nsManager, _, _, _ string) (*net.IPNet, *net.HardwareAddr, error) {
	return nil, nil, errUnsupportedPlatform
}

func bringUpLoopback(_ string) error {
	return errUnsupportedPlatform
}

func checkLoopback(_ string) error {
	return errUnsupportedPlatform
}
