package server

import (
	"fmt"
	"io"
	"io/ioutil"

	"github.com/containers/storage/pkg/pools"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// PortForward prepares a streaming endpoint to forward ports from a PodSandbox.
func (s *Server) PortForward(ctx context.Context, req *pb.PortForwardRequest) (*pb.PortForwardResponse, error) {
	resp, err := s.getPortForward(req)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare portforward endpoint")
	}

	return resp, nil
}

func (s StreamService) PortForward(podSandboxID string, port int32, stream io.ReadWriteCloser) error {
	ctx := log.AddRequestNameAndID(context.Background(), "PortForward")

	// if we error in this function before Copying all of the content out of the stream,
	// this stream will eventually get full, which causes leakages and can eventually brick CRI-O
	// ref https://bugzilla.redhat.com/show_bug.cgi?id=1798193
	emptyStreamOnError := true
	defer func() {
		if emptyStreamOnError && stream != nil {
			go func() {
				_, copyError := pools.Copy(ioutil.Discard, stream)
				log.Errorf(ctx, "error closing port forward stream after other error: %v", copyError)
			}()
		}
	}()

	sandboxID, err := s.runtimeServer.PodIDIndex().Get(podSandboxID)
	if err != nil {
		return fmt.Errorf("PodSandbox with ID starting with %s not found: %v", podSandboxID, err)
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
		return errors.Errorf(
			"network namespace path of sandbox %s is empty", sb.ID(),
		)
	}

	// defer responsibility of emptying stream to PortForwardContainer
	emptyStreamOnError = false

	return s.runtimeServer.Runtime().PortForwardContainer(ctx, sb.InfraContainer(), netNsPath, port, stream)
}
