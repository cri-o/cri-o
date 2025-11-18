//go:build linux

package server

import (
	"context"
	"fmt"
	"os"

	"github.com/containernetworking/plugins/pkg/ns"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
)

// validateNetworkNamespace checks if the given path is a valid network namespace.
func (s *Server) validateNetworkNamespace(netnsPath string) error {
	netns, err := ns.GetNS(netnsPath)
	if err != nil {
		return fmt.Errorf("invalid network namespace: %w", err)
	}

	defer netns.Close()

	return nil
}

// cleanupNetns removes a network namespace file and logs the action.
func (s *Server) cleanupNetns(ctx context.Context, netnsPath string, sb *sandbox.Sandbox) {
	if rmErr := os.RemoveAll(netnsPath); rmErr != nil {
		log.Warnf(ctx, "Failed to remove netns path %s: %v", netnsPath, rmErr)
	} else {
		log.Infof(ctx, "Removed netns path %s from pod sandbox %s(%s)", netnsPath, sb.Name(), sb.ID())
	}
}
