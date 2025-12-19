package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"go.podman.io/storage/pkg/pools"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib"
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

	// Check if this port should use reverse mode based on pod annotations
	// The annotation "io.cri-o.reverse-ports" contains a comma-separated list of ports
	// that should be reverse forwarded (e.g., "8080,9090")
	reverse := s.isReversePort(sb, port)

	return s.runtimeServer.ContainerServer.Runtime().PortForwardContainer(ctx, sb.InfraContainer(), netNsPath, port, stream, reverse)
}

// isReversePort checks if the specified port should use reverse port forwarding
// based on the pod's annotations.
func (s *StreamService) isReversePort(sb *lib.Sandbox, port int32) bool {
	annotations := sb.Annotations()
	if annotations == nil {
		return false
	}

	// Check for the reverse ports annotation
	reversePorts, ok := annotations["io.cri-o.reverse-ports"]
	if !ok {
		return false
	}

	// Parse the comma-separated list of ports
	for _, portStr := range strings.Split(reversePorts, ",") {
		portStr = strings.TrimSpace(portStr)
		if portStr == "" {
			continue
		}

		// Parse the port number
		reversePort, err := strconv.ParseInt(portStr, 10, 32)
		if err != nil {
			log.Warnf(context.Background(), "Invalid port in reverse-ports annotation: %s", portStr)
			continue
		}

		if int32(reversePort) == port {
			return true
		}
	}

	return false
}
