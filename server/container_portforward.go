package server

import (
	"fmt"
	"io"
	"io/ioutil"
	"time"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/docker/docker/pkg/pools"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// PortForward prepares a streaming endpoint to forward ports from a PodSandbox.
func (s *Server) PortForward(ctx context.Context, req *pb.PortForwardRequest) (resp *pb.PortForwardResponse, err error) {
	const operation = "port_forward"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	resp, err = s.getPortForward(req)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare portforward endpoint")
	}

	return resp, nil
}

func (ss StreamService) PortForward(podSandboxID string, port int32, stream io.ReadWriteCloser) error {
	// if we error in this function before Copying all of the content out of the stream,
	// this stream will eventually get full, which causes leakages and can eventually brick CRI-O
	// ref https://bugzilla.redhat.com/show_bug.cgi?id=1798193
	emptyStreamOnError := true
	defer func() {
		if emptyStreamOnError && stream != nil {
			go func() {
				_, copyError := pools.Copy(ioutil.Discard, stream)
				logrus.Errorf("error closing port forward stream after other error: %v", copyError)
			}()
		}
	}()

	sandboxID, err := ss.runtimeServer.PodIDIndex().Get(podSandboxID)
	if err != nil {
		return fmt.Errorf("PodSandbox with ID starting with %s not found: %v", podSandboxID, err)
	}
	c := ss.runtimeServer.GetSandboxContainer(sandboxID)

	if c == nil {
		return fmt.Errorf("could not find container for sandbox %q", podSandboxID)
	}

	if err := ss.runtimeServer.Runtime().UpdateContainerStatus(c); err != nil {
		return err
	}

	cState := c.State()
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return fmt.Errorf("container is not created or running")
	}

	emptyStreamOnError = false
	return ss.runtimeServer.Runtime().PortForwardContainer(c, port, stream)
}
