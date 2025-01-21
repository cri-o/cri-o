package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	cri "k8s.io/cri-client/pkg"
)

func (s *Server) checkCRIHealth(ctx context.Context, timeout time.Duration) error {
	// Validate that a CRI connection is possible using the socket path.
	rrs, err := cri.NewRemoteRuntimeService(s.ContainerServer.Config().Listen, timeout, nil, nil)
	if err != nil {
		return fmt.Errorf("create remote runtime service: %w", err)
	}

	// Retrieve the runtime status using the socket
	response, err := rrs.Status(ctx, false)
	if err != nil {
		return fmt.Errorf("get runtime status: %w", err)
	}

	// Verify that everything is okay
	if response.GetStatus() == nil {
		return errors.New("runtime status is nil")
	}

	if response.GetStatus().GetConditions() == nil {
		return errors.New("runtime conditions are nil")
	}

	for _, c := range response.GetStatus().GetConditions() {
		if !c.GetStatus() {
			return fmt.Errorf(
				"runtime status %q is invalid: %s (reason: %s)",
				c.GetType(), c.GetMessage(), c.GetReason(),
			)
		}
	}

	return nil
}
