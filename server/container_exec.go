package server

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	utilexec "k8s.io/kubernetes/pkg/util/exec"
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
func (ss streamService) Exec(containerID string, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan term.Size) error {
	fmt.Println(containerID, cmd, stdin, stdout, stderr, tty, resize)
	c := ss.runtimeServer.state.containers.Get(containerID)

	if err := ss.runtimeServer.runtime.UpdateStatus(c); err != nil {
		return err
	}

	cState := ss.runtimeServer.runtime.ContainerStatus(c)
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return fmt.Errorf("container is not created or running")
	}

	args := []string{"exec"}
	if tty {
		args = append(args, "-t")
	}
	args = append(args, c.Name)
	args = append(args, cmd...)
	execCmd := exec.Command(ss.runtimeServer.runtime.Path(c), args...)
	var cmdErr error
	if tty {
		p, err := kubecontainer.StartPty(execCmd)
		if err != nil {
			return err
		}
		defer p.Close()

		// make sure to close the stdout stream
		defer stdout.Close()

		kubecontainer.HandleResizing(resize, func(size term.Size) {
			term.SetSize(p.Fd(), size)
		})

		if stdin != nil {
			go io.Copy(p, stdin)
		}

		if stdout != nil {
			go io.Copy(stdout, p)
		}

		cmdErr = execCmd.Wait()
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
			go io.Copy(w, stdin)

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
