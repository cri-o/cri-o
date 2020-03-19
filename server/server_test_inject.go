// +build test
// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package server

import (
	"github.com/cri-o/ocicni/pkg/ocicni"
)

// SetNetPlugin sets the network plugin for the ContainerServer. The function
// errors if a sane shutdown of the initially created network plugin failed.
func (s *Server) SetNetPlugin(plugin ocicni.CNIPlugin) error {
	if err := s.netPlugin.Shutdown(); err != nil {
		return err
	}
	s.netPlugin = plugin
	return nil
}
