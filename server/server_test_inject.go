//go:build test
// +build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package server

import (
	"github.com/cri-o/ocicni/pkg/ocicni"
)

// SetStorageRuntimeServer sets the runtime server for the ContainerServer
func (s *StreamService) SetRuntimeServer(server *Server) {
	s.runtimeServer = server
}

// SetCNIPlugin sets the network plugin for the ContainerServer. The function
// errors if a sane shutdown of the initially created network plugin failed.
func (s *Server) SetCNIPlugin(plugin ocicni.CNIPlugin) error {
	return s.config.SetCNIPlugin(plugin)
}
