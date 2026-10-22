//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package server

import "context"

// SetStorageRuntimeServer sets the runtime server for the ContainerServer.
func (s *StreamService) SetRuntimeServer(server *Server) {
	s.runtimeServer = server
}

// MonitorStreamingURL returns the URL of the container monitor for a
// streaming request, or an empty one if CRI-O's streaming server serves it.
func MonitorStreamingURL(
	ctx context.Context,
	enabled func(runtimeHandler string) (bool, error),
	runtimeHandler, request, target string,
	serve func() (string, error),
) (string, error) {
	return monitorStreamingURL(ctx, enabled, runtimeHandler, request, target, serve)
}
