//go:build !linux && !freebsd

package server

import (
	"context"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
)

// validateNetworkNamespace checks if the given path is a valid network namespace
// On unsupported platforms, this is a no-op since network namespaces are Linux-specific.
func (s *Server) validateNetworkNamespace(netnsPath string) error {
	return nil
}

// cleanupNetns removes a network namespace file and logs the action
// On unsupported platforms, this is a no-op since network namespaces are Linux-specific.
func (s *Server) cleanupNetns(ctx context.Context, netnsPath string, sb *sandbox.Sandbox) {
	log.Debugf(ctx, "Network namespace cleanup not supported on this platform")
}
