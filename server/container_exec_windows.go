// +build windows

package server

import (
	"fmt"
	"io"
	"os/exec"

	"k8s.io/client-go/tools/remotecommand"
)

func (ss streamService) ttyCmd(cmd *exec.Cmd, stdin io.Reader, stdout io.WriteCloser, resize <-chan remotecommand.TerminalSize) error {
	return fmt.Errorf("unsupported")
}
