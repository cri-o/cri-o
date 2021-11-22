//go:build !windows
// +build !windows

package server

import "net"

// Listen opens the network address for the server. Expects the config.Listen address.
//
// This is a platform specific wrapper.
func Listen(network, address string) (net.Listener, error) {
	return net.Listen(network, address)
}
