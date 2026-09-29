//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package server

import "github.com/cri-o/cri-o/internal/securityprofile"

// SetStorageRuntimeServer sets the runtime server for the ContainerServer.
func (s *StreamService) SetRuntimeServer(server *Server) {
	s.runtimeServer = server
}

// SecurityProfiles returns the store of the security profiles.
func (s *Server) SecurityProfiles() *securityprofile.Store {
	return s.securityProfiles
}
