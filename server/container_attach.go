package server

// #include "../conmon/config.h"
import "C"

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/utils"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	"k8s.io/client-go/tools/remotecommand"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

// Attach prepares a streaming endpoint to attach to a running container.
func (s *Server) Attach(ctx context.Context, req *pb.AttachRequest) (resp *pb.AttachResponse, err error) {
	const operation = "attach"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()
	logrus.Debugf("AttachRequest %+v", req)

	resp, err = s.GetAttach(req)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare attach endpoint")
	}

	return resp, nil
}

// Attach endpoint for streaming.Runtime
func (ss streamService) Attach(containerID string, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	c := ss.runtimeServer.GetContainer(containerID)

	if c == nil {
		return fmt.Errorf("could not find container %q", containerID)
	}

	if err := ss.runtimeServer.Runtime().UpdateStatus(c); err != nil {
		return err
	}

	cState := ss.runtimeServer.Runtime().ContainerStatus(c)
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return fmt.Errorf("container is not created or running")
	}

	controlPath := filepath.Join(c.BundlePath(), "ctl")
	controlFile, err := os.OpenFile(controlPath, unix.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open container ctl file: %v", err)
	}
	defer controlFile.Close()

	kubecontainer.HandleResizing(resize, func(size remotecommand.TerminalSize) {
		logrus.Infof("Got a resize event: %+v", size)
		_, err := fmt.Fprintf(controlFile, "%d %d %d\n", 1, size.Height, size.Width)
		if err != nil {
			logrus.Infof("Failed to write to control file to resize terminal: %v", err)
		}
	})

	attachSocketPath := filepath.Join(oci.ContainerAttachSocketDir, c.ID(), "attach")
	conn, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: attachSocketPath, Net: "unixpacket"})
	if err != nil {
		return fmt.Errorf("failed to connect to container %s attach socket: %v", c.ID(), err)
	}
	defer conn.Close()

	receiveStdout := make(chan error)
	if outputStream != nil || errorStream != nil {
		go func() {
			receiveStdout <- redirectResponseToOutputStreams(outputStream, errorStream, conn)
		}()
	}

	stdinDone := make(chan error)
	go func() {
		var err error
		if inputStream != nil {
			_, err = utils.CopyDetachable(conn, inputStream, nil)
			conn.CloseWrite()
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

func redirectResponseToOutputStreams(outputStream, errorStream io.Writer, conn io.Reader) error {
	stdioBufSize := int(C.STDIO_BUF_SIZE)
	var err error
	buf := make([]byte, stdioBufSize+1)

	for {
		nr, er := conn.Read(buf)
		if nr > 0 {
			var dst io.Writer
			if buf[0] == byte(C.STDOUT_PIPE) {
				dst = outputStream
			} else if buf[0] == byte(C.STDERR_PIPE) {
				dst = errorStream
			} else {
				logrus.Infof("Got unexpected attach type %+d", buf[0])
			}

			if dst != nil {
				nw, ew := dst.Write(buf[1:nr])
				if ew != nil {
					err = ew
					break
				}
				if nr != nw+1 {
					err = io.ErrShortWrite
					break
				}
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			err = er
			break
		}
	}

	return err
}
