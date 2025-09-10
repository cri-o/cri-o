package server

import (
	"context"
	"errors"
	"fmt"
	"io"

	"go.podman.io/storage/pkg/pools"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
)

// PortForward prepares a streaming endpoint to forward ports from a PodSandbox.
func (s *Server) PortForward(ctx context.Context, req *types.PortForwardRequest) (*types.PortForwardResponse, error) {
	resp, err := s.getPortForward(req)
	if err != nil {
		return nil, errors.New("unable to prepare portforward endpoint")
	}

	return resp, nil
}

func (s *StreamService) PortForward(ctx context.Context, podSandboxID string, port int32, stream io.ReadWriteCloser) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	// Drain the stream to prevent failure to close the connection and memory leakage.
	// ref https://bugzilla.redhat.com/show_bug.cgi?id=1798193
	// ref https://issues.redhat.com/browse/OCPBUGS-30978
	defer func() {
		if stream == nil {
			return
		}

		go func() {
			if _, err := pools.Copy(io.Discard, stream); err != nil {
				log.Errorf(ctx, "Unable to drain the stream data: %v", err)
			}
		}()
	}()

	sandboxID, err := s.runtimeServer.ContainerServer.PodIDIndex().Get(podSandboxID)
	if err != nil {
		return fmt.Errorf("PodSandbox with ID starting with %s not found: %w", podSandboxID, err)
	}

	sb := s.runtimeServer.GetSandbox(sandboxID)
	if sb == nil {
		return fmt.Errorf("could not find sandbox %s", podSandboxID)
	}

	if !sb.Ready() {
		return fmt.Errorf("sandbox %s is not running", podSandboxID)
	}

	netNsPath := sb.NetNsPath()
	if netNsPath == "" {
		return fmt.Errorf(
			"network namespace path of sandbox %s is empty", sb.ID(),
		)
	}

	return s.runtimeServer.ContainerServer.Runtime().PortForwardContainer(ctx, sb.InfraContainer(), netNsPath, port, stream)
}
