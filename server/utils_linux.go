// +build linux

package server

import (
	"github.com/opencontainers/runtime-tools/validate"
	"github.com/syndtr/gocapability/capability"
)

func lastCapability() capability.Cap {
	return validate.LastCap()
}
