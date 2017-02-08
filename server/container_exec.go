package server

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	"k8s.io/kubernetes/pkg/util/term"
)

// Exec prepares a streaming endpoint to execute a command in the container.
func (s *Server) Exec(ctx context.Context, req *pb.ExecRequest) (*pb.ExecResponse, error) {
	logrus.Debugf("ExecRequest %+v", req)

	resp, err := s.GetExec(req)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare exec endpoint")
	}

	return resp, nil
}

// Exec endpoint for streaming.Runtime
func (ss streamService) Exec(containerID string, cmd []string, in io.Reader, out, errOut io.WriteCloser, tty bool, resize <-chan term.Size) error {
	fmt.Println(containerID, cmd, in, out, errOut, tty, resize)
	c := ss.runtimeServer.state.containers.Get(containerID)

	if err := ss.runtimeServer.runtime.UpdateStatus(c); err != nil {
		return err
	}

	cState := ss.runtimeServer.runtime.ContainerStatus(c)
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return fmt.Errorf("container is not created or running")
	}

	args := []string{"exec", c.Name()}                                 // exec ctr
	args = append(args, cmd...)                                        // exec ctr cmd
	execCmd := exec.Command(ss.runtimeServer.runtime.Path(c), args...) // runc exec ctr cmd
	execCmd.Stdin = in
	execCmd.Stdout = out
	execCmd.Stderr = errOut

	if err := execCmd.Run(); err != nil {
		return err
	}

	return nil
}
