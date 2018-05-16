// +build !windows

package server

import (
	"io"
	"os/exec"

	"github.com/docker/docker/pkg/pools"
	"github.com/kr/pty"
	"k8s.io/client-go/tools/remotecommand"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"k8s.io/kubernetes/pkg/util/term"
)

func (ss streamService) ttyCmd(execCmd *exec.Cmd, stdin io.Reader, stdout io.WriteCloser, resize <-chan remotecommand.TerminalSize) error {
	p, err := pty.Start(execCmd)
	if err != nil {
		return err
	}
	defer p.Close()

	// make sure to close the stdout stream
	defer stdout.Close()

	kubecontainer.HandleResizing(resize, func(size remotecommand.TerminalSize) {
		term.SetSize(p.Fd(), size)
	})

	if stdin != nil {
		go pools.Copy(p, stdin)
	}

	if stdout != nil {
		go pools.Copy(stdout, p)
	}

	return execCmd.Wait()
}
