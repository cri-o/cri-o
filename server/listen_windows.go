package server

import (
	"net"

	"github.com/Microsoft/go-winio"
)

// Listen opens the network address for the server. Expects the config.Listen address.
//
// This is a platform specific wrapper.
func Listen(network, address string) (net.Listener, error) {
	if network == "unix" {
		return winio.ListenPipe(address, nil)
	}
	return net.Listen(network, address)
}
