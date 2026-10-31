package server

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strconv"

	"go.podman.io/storage/pkg/pools"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
)

// PortForward prepares a streaming endpoint to forward ports from a PodSandbox.
func (s *Server) PortForward(
	ctx context.Context,
	req *types.PortForwardRequest,
) (*types.PortForwardResponse, error) {
	sb, err := s.getPodSandboxFromRequest(ctx, req.GetPodSandboxId())
	if err != nil {
		return nil, fmt.Errorf("could not find pod sandbox %q: %w", req.GetPodSandboxId(), err)
	}

	streamURL, err := monitorStreamingURL(
		ctx,
		s.Runtime().RuntimeStreamPortForward,
		sb.RuntimeHandler(),
		"port forward",
		fmt.Sprintf("pod sandbox %q", req.GetPodSandboxId()),
		func() (string, error) { return s.servePortForward(ctx, sb, req.GetPort()) },
	)
	if err != nil {
		return nil, err
	}

	if streamURL != "" {
		return &types.PortForwardResponse{Url: streamURL}, nil
	}

	resp, err := s.getPortForward(req)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare port forward endpoint: %w", err)
	}

	return resp, nil
}

// servePortForward returns the port forward URL of the container monitor of
// the sandbox for the ports.
func (s *Server) servePortForward(
	ctx context.Context,
	sb *sandbox.Sandbox,
	ports []int32,
) (string, error) {
	netNsPath, err := portForwardNetNsPath(sb)
	if err != nil {
		return "", err
	}

	monitorURL, err := s.Runtime().ServePortForwardContainer(ctx, sb.InfraContainer(), netNsPath)
	if err != nil {
		return "", err
	}

	return portForwardURL(monitorURL, ports)
}

// portForwardURL returns the port forward URL of the container monitor with
// the ports in its "port" query parameter. The WebSocket channel protocol of
// the container monitor takes the ports from there, since the kubelet does
// not pass the query of its client on, while CRI-O's streaming server keeps
// them with the request. The SPDY protocols ignore the query.
func portForwardURL(monitorURL string, ports []int32) (string, error) {
	if len(ports) == 0 {
		return monitorURL, nil
	}

	u, err := url.Parse(monitorURL)
	if err != nil {
		return "", fmt.Errorf("parse port forward URL %q: %w", monitorURL, err)
	}

	query := u.Query()
	for _, port := range ports {
		query.Add("port", strconv.FormatInt(int64(port), 10))
	}

	u.RawQuery = query.Encode()

	return u.String(), nil
}

// portForwardNetNsPath returns the network namespace path to forward the
// ports of the sandbox in, which has to be running.
func portForwardNetNsPath(sb *sandbox.Sandbox) (string, error) {
	if !sb.Ready() {
		return "", fmt.Errorf("sandbox %s is not running", sb.ID())
	}

	netNsPath := sb.NetNsPath()
	if netNsPath == "" {
		return "", fmt.Errorf(
			"network namespace path of sandbox %s is empty", sb.ID(),
		)
	}

	return netNsPath, nil
}

func (s *StreamService) PortForward(
	ctx context.Context,
	podSandboxID string,
	port int32,
	stream io.ReadWriteCloser,
) error {
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

	netNsPath, err := portForwardNetNsPath(sb)
	if err != nil {
		return err
	}

	return s.runtimeServer.ContainerServer.Runtime().
		PortForwardContainer(ctx, sb.InfraContainer(), netNsPath, port, stream)
}
