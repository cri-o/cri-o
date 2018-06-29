// +build !linux

package ocicni

import (
	"fmt"
	"net"
)

type nsManager struct {
}

func (nsm *nsManager) init() error {
	return nil
}

func getContainerIP(nsm *nsManager, netnsPath, interfaceName, addrType string) (net.IP, error) {
	return nil, fmt.Errorf("not supported yet")
}
