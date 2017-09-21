package libkpod

import (
	"github.com/docker/docker/pkg/term"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/utils"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"k8s.io/client-go/tools/remotecommand"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"

	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

/* Sync with stdpipe_t in conmon.c */
const (
	AttachPipeStdin  = 1
	AttachPipeStdout = 2
	AttachPipeStderr = 3
)

// ContainerAttach attaches to a running container
// keys are string representation of what to use to detach
// key format is a single character [a-Z] or ctrl-<value> where <value> is one of: a-z, @, ^, [, , or _.
func (c *ContainerServer) ContainerAttach(container string, noStdIn bool, keys string) error {
	// Check the validity of the provided keys first
	var err error
	detachKeys := []byte{}
	if len(keys) > 0 {
		detachKeys, err = term.ToBytes(keys)
		if err != nil {
			return errors.Wrapf(err, "invalid detach keys")
		}
	}

	ctr, err := c.LookupContainer(container)
	if err != nil {
		return errors.Wrapf(err, "failed to find container %s", container)
	}

	cStatus := c.runtime.ContainerStatus(ctr)
	if !(cStatus.Status == oci.ContainerStateRunning || cStatus.Status == oci.ContainerStateCreated) {
		return errors.Errorf("%s is not created or running", container)
	}

	resize := make(chan remotecommand.TerminalSize)
	defer close(resize)
	AttachContainerSocket(ctr, resize, noStdIn, detachKeys)
	c.ContainerStateToDisk(ctr)

	return nil
}

// AttachContainerSocket connects to the container's attach socket and deals with the IO
func AttachContainerSocket(ctr *oci.Container, resize <-chan remotecommand.TerminalSize, noStdIn bool, detachKeys []byte) error {
	inputStream := os.Stdin
	outputStream := os.Stdout
	errorStream := os.Stderr

	defer inputStream.Close()

	tty, err := strconv.ParseBool(ctr.State().Annotations["io.kubernetes.cri-o.TTY"])
	if err != nil {
		return errors.Wrapf(err, "unable to parse annotations in %s", ctr.ID)
	}
	if !tty {
		return errors.Errorf("no tty available for %s", ctr.ID())
	}

	oldTermState, err := term.SaveState(inputStream.Fd())

	if err != nil {
		return errors.Wrapf(err, "unable to save terminal state")
	}

	defer term.RestoreTerminal(inputStream.Fd(), oldTermState)

	// Put both input and output into raw
	if !noStdIn {
		term.SetRawTerminal(inputStream.Fd())
	}

	controlPath := filepath.Join(ctr.BundlePath(), "ctl")
	controlFile, err := os.OpenFile(controlPath, unix.O_WRONLY, 0)
	if err != nil {
		return errors.Wrapf(err, "failed to open container ctl file: %v")
	}

	kubecontainer.HandleResizing(resize, func(size remotecommand.TerminalSize) {
		logrus.Debug("Got a resize event: %+v", size)
		_, err := fmt.Fprintf(controlFile, "%d %d %d\n", 1, size.Height, size.Width)
		if err != nil {
			logrus.Warn("Failed to write to control file to resize terminal: %v", err)
		}
	})
	attachSocketPath := filepath.Join("/var/run/crio", ctr.ID(), "attach")
	conn, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: attachSocketPath, Net: "unixpacket"})
	if err != nil {
		return errors.Wrapf(err, "failed to connect to container's attach socket: %v")
	}
	defer conn.Close()

	receiveStdoutError := make(chan error)
	if outputStream != nil || errorStream != nil {
		go func() {
			receiveStdoutError <- redirectResponseToOutputStreams(outputStream, errorStream, conn)
		}()
	}

	stdinDone := make(chan error)
	go func() {
		var err error
		if inputStream != nil && !noStdIn {
			_, err = utils.CopyDetachable(conn, inputStream, detachKeys)
			conn.CloseWrite()
		}
		stdinDone <- err
	}()

	select {
	case err := <-receiveStdoutError:
		return err
	case err := <-stdinDone:
		if _, ok := err.(utils.DetachError); ok {
			return nil
		}
		if outputStream != nil || errorStream != nil {
			return <-receiveStdoutError
		}
	}

	return nil
}

func redirectResponseToOutputStreams(outputStream, errorStream io.Writer, conn io.Reader) error {
	var err error
	buf := make([]byte, 8192+1) /* Sync with conmon STDIO_BUF_SIZE */
	for {
		nr, er := conn.Read(buf)
		if nr > 0 {
			var dst io.Writer
			switch buf[0] {
			case AttachPipeStdout:
				dst = outputStream
			case AttachPipeStderr:
				dst = errorStream
			default:
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
