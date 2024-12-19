//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package server

// SetStorageRuntimeServer sets the runtime server for the ContainerServer.
func (s *StreamService) SetRuntimeServer(server *Server) {
	s.runtimeServer = server
}
