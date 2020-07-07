package server

import (
	"fmt"
	"io"
	"io/ioutil"

	"github.com/containers/storage/pkg/pools"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
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

	c := sb.InfraContainer()
	if err := s.runtimeServer.Runtime().UpdateContainerStatus(c); err != nil {
		return err
	}

	cState := c.State()
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return fmt.Errorf("container is not created or running")
	}

	emptyStreamOnError = false

	if sb.NetNsPath() == "" {
		return errors.Errorf(
			"network namespace path of sandbox %s is empty", sb.ID(),
		)
	}

	return s.runtimeServer.Runtime().PortForwardContainer(ctx, c, sb.NetNsPath(), port, stream)
}
