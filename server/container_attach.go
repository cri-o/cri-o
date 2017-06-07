package server

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/utils"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"k8s.io/kubernetes/pkg/util/term"
)

// Attach prepares a streaming endpoint to attach to a running container.
func (s *Server) Attach(ctx context.Context, req *pb.AttachRequest) (*pb.AttachResponse, error) {
	logrus.Debugf("AttachRequest %+v", req)

	resp, err := s.GetAttach(req)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare attach endpoint")
	}

	return resp, nil
}

// Attach endpoint for streaming.Runtime
func (ss streamService) Attach(containerID string, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resize <-chan term.Size) error {
	c := ss.runtimeServer.GetContainer(containerID)

	if c == nil {
		return fmt.Errorf("could not find container %q", containerID)
	}

	if err := ss.runtimeServer.runtime.UpdateStatus(c); err != nil {
		return err
	}

	cState := ss.runtimeServer.runtime.ContainerStatus(c)
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return fmt.Errorf("container is not created or running")
	}

	controlPath := filepath.Join(c.BundlePath(), "ctl")
	controlFile, err := os.OpenFile(controlPath, syscall.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open container ctl file: %v", err)
	}

	kubecontainer.HandleResizing(resize, func(size term.Size) {
		logrus.Infof("Got a resize event: %+v", size)
		_, err := fmt.Fprintf(controlFile, "%d %d %d\n", 1, size.Height, size.Width)
		if err != nil {
			logrus.Infof("Failed to write to control file to resize terminal: %v", err)
		}
	})

	attachSocketPath := filepath.Join("/var/run/crio", c.ID(), "attach")
	conn, err := net.Dial("unix", attachSocketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to container %s attach socket: %v", c.ID(), err)
	}
	defer conn.Close()

	receiveStdout := make(chan error)
	if outputStream != nil || errorStream != nil {
		go func() {
			receiveStdout <- redirectResponseToOutputStream(tty, outputStream, errorStream, conn)
		}()
	}

	stdinDone := make(chan error)
	go func() {
		var err error
		if inputStream != nil {
			_, err = utils.CopyDetachable(conn, inputStream, nil)
		}
		stdinDone <- err
	}()

	select {
	case err := <-receiveStdout:
		return err
	case err := <-stdinDone:
		if _, ok := err.(utils.DetachError); ok {
			return nil
		}
		if outputStream != nil || errorStream != nil {
			return <-receiveStdout
		}
	}

	return nil
}

func redirectResponseToOutputStream(tty bool, outputStream, errorStream io.Writer, conn io.Reader) error {
	if outputStream == nil {
		outputStream = ioutil.Discard
	}
	if errorStream == nil {
		// errorStream = ioutil.Discard
	}
	var err error
	if tty {
		_, err = io.Copy(outputStream, conn)
	} else {
		// TODO
	}
	return err
}
