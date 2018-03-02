// +build !linux

package server

import "github.com/syndtr/gocapability/capability"

func lastCapability() capability.Cap {
	return capability.Cap(-1)
}
