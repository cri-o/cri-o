package server

import (
	"errors"
	"fmt"
	"io"

	"github.com/containers/storage/pkg/pools"
	"github.com/cri-o/cri-o/internal/log"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// PortForward prepares a streaming endpoint to forward ports from a PodSandbox.
func (s *Server) PortForward(ctx context.Context, req *types.PortForwardRequest) (*types.PortForwardResponse, error) {
	resp, err := s.getPortForward(req)
	if err != nil {
		return nil, errors.New("unable to prepare portforward endpoint")
	}

	return resp, nil
}

func (s StreamService) PortForward(ctx context.Context, podSandboxID string, port int32, stream io.ReadWriteCloser) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	// if we error in this function before Copying all of the content out of the stream,
	// this stream will eventually get full, which causes leakages and can eventually brick CRI-O
	// ref https://bugzilla.redhat.com/show_bug.cgi?id=1798193
	emptyStreamOnError := true
	defer func() {
		if emptyStreamOnError && stream != nil {
			go func() {
				_, copyError := pools.Copy(io.Discard, stream)
				log.Errorf(ctx, "Error closing port forward stream after other error: %v", copyError)
			}()
		}
	}()

	sandboxID, err := s.runtimeServer.PodIDIndex().Get(podSandboxID)
	if err != nil {
		return fmt.Errorf("PodSandbox with ID starting with %s not found: %w", podSandboxID, err)
	}

	sb := s.runtimeServer.GetSandbox(sandboxID)
	if sb == nil {
		return fmt.Errorf("could not find sandbox %s", podSandboxID)
	}

	if !sb.Ready(true) {
		return fmt.Errorf("sandbox %s is not running", podSandboxID)
	}

	netNsPath := sb.NetNsPath()
	if netNsPath == "" {
		return fmt.Errorf(
			"network namespace path of sandbox %s is empty", sb.ID(),
		)
	}

	// defer responsibility of emptying stream to PortForwardContainer
	emptyStreamOnError = false

	return s.runtimeServer.Runtime().PortForwardContainer(ctx, sb.InfraContainer(), netNsPath, port, stream)
}
