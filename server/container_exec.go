package server

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/docker/docker/pkg/pools"
	"github.com/kubernetes-sigs/cri-o/oci"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"k8s.io/client-go/tools/remotecommand"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
	utilexec "k8s.io/utils/exec"
)

// Exec prepares a streaming endpoint to execute a command in the container.
func (s *Server) Exec(ctx context.Context, req *pb.ExecRequest) (resp *pb.ExecResponse, err error) {
	const operation = "exec"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	logrus.Debugf("ExecRequest %+v", req)

	resp, err = s.getExec(req)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare exec endpoint: %v", err)
	}

	return resp, nil
}

// Exec endpoint for streaming.Runtime
func (ss streamService) Exec(containerID string, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	c, err := ss.runtimeServer.GetContainerFromShortID(containerID)
	if err != nil {
		return fmt.Errorf("could not find container %q: %v", containerID, err)
	}

	if err := ss.runtimeServer.Runtime().UpdateStatus(c); err != nil {
		return err
	}

	cState := ss.runtimeServer.Runtime().ContainerStatus(c)
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return fmt.Errorf("container is not created or running")
	}

	processFile, err := oci.PrepareProcessExec(c, cmd, tty)
	if err != nil {
		return err
	}
	defer os.RemoveAll(processFile.Name())

	rPath, err := ss.runtimeServer.Runtime().Path(c)
	if err != nil {
		return err
	}

	args := []string{"exec"}
	args = append(args, "--process", processFile.Name())
	args = append(args, c.ID())
	execCmd := exec.Command(rPath, args...)
	var cmdErr error
	if tty {
		cmdErr = ss.ttyCmd(execCmd, stdin, stdout, resize)
	} else {
		if stdin != nil {
			// Use an os.Pipe here as it returns true *os.File objects.
			// This way, if you run 'kubectl exec <pod> -i bash' (no tty) and type 'exit',
			// the call below to execCmd.Run() can unblock because its Stdin is the read half
			// of the pipe.
			r, w, err := os.Pipe()
			if err != nil {
				return err
			}
			go pools.Copy(w, stdin)

			execCmd.Stdin = r
		}
		if stdout != nil {
			execCmd.Stdout = stdout
		}
		if stderr != nil {
			execCmd.Stderr = stderr
		}

		cmdErr = execCmd.Run()
	}

	if exitErr, ok := cmdErr.(*exec.ExitError); ok {
		return &utilexec.ExitErrorWrapper{ExitError: exitErr}
	}
	return cmdErr
}
