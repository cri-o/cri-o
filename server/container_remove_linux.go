package server

import (
	"context"

	"github.com/cri-o/cri-o/internal/config/seccomp"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
)

func (s *Server) removeSeccompNotifier(ctx context.Context, c *oci.Container) {
	if notifier, ok := s.seccompNotifiers.Load(c.ID()); ok {
		n, ok := notifier.(*seccomp.Notifier)
		if ok {
			if err := n.Close(); err != nil {
				log.Errorf(ctx, "Unable to close seccomp notifier: %v", err)
			}
		}
	}
}
