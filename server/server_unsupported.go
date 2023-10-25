//go:build !linux
// +build !linux

package server

import (
	"context"
)

func (s *Server) startSeccompNotifierWatcher(ctx context.Context) error {
	return nil
}
