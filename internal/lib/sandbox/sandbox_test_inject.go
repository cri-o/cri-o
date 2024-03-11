//go:build test
// +build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package sandbox

import (
	"github.com/cri-o/cri-o/internal/hostport"
)

// SetPortMappings sets the PortMappings for the Sandbox
func (s *Sandbox) SetPortMappings(portMappings []*hostport.PortMapping) {
	s.portMappings = portMappings
}

func (s *Sandbox) SetResolvPath(resolvPath string) {
	s.resolvPath = resolvPath
}

func (s *Sandbox) SetHostnamePath(hostnamePath string) {
	s.hostnamePath = hostnamePath
}

func (s *Sandbox) SetContainerEnvPath(containerEnvPath string) {
	s.containerEnvPath = containerEnvPath
}

func (s *Sandbox) SetShmPath(shmPath string) {
	s.shmPath = shmPath
}

func (s *Sandbox) SetHostNetwork(hostNetwork bool) {
	s.hostNetwork = hostNetwork
}
