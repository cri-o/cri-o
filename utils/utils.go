package utils

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Sirupsen/logrus"
)

const PR_SET_CHILD_SUBREAPER = 36

func ExecCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("`%v %v` failed: %v (%v)", name, strings.Join(args, " "), stderr.String(), err)
	}

	return stdout.String(), nil
}

// ExecCmdWithStdStreams execute a command with the specified standard streams.
func ExecCmdWithStdStreams(stdin, stdout, stderr *os.File, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("`%v %v` failed: %v", name, strings.Join(args, " "), err)
	}

	return nil
}

// SetSubreaper sets the value i as the subreaper setting for the calling process
func SetSubreaper(i int) error {
	return Prctl(PR_SET_CHILD_SUBREAPER, uintptr(i), 0, 0, 0)
}

// Prctl is a way to make the prctl linux syscall
func Prctl(option int, arg2, arg3, arg4, arg5 uintptr) (err error) {
	_, _, e1 := syscall.Syscall6(syscall.SYS_PRCTL, uintptr(option), arg2, arg3, arg4, arg5, 0)
	if e1 != 0 {
		err = e1
	}
	return
}

// CreateFakeRootfs creates a fake rootfs for test.
func CreateFakeRootfs(dir string, image string) error {
	if len(image) <= 9 || image[:9] != "docker://" {
		return fmt.Errorf("CreateFakeRootfs only support docker images currently")
	}

	rootfs := filepath.Join(dir, "rootfs")
	if err := os.MkdirAll(rootfs, 0755); err != nil {
		return err
	}

	// docker export $(docker create image[9:]) | tar -C rootfs -xf -
	return dockerExport(image[9:], rootfs)
}

func dockerExport(image string, rootfs string) error {
	out, err := ExecCmd("docker", "create", image)
	if err != nil {
		return err
	}

	container := out[:strings.Index(out, "\n")]

	cmd := fmt.Sprintf("docker export %s | tar -C %s -xf -", container, rootfs)
	if _, err := ExecCmd("/bin/bash", "-c", cmd); err != nil {
		err1 := dockerRemove(container)
		if err1 == nil {
			return err
		}
		return fmt.Errorf("%v; %v", err, err1)
	}

	return dockerRemove(container)
}

func dockerRemove(container string) error {
	if _, err := ExecCmd("docker", "rm", container); err != nil {
		return err
	}
	return nil
}

// StartReaper starts a goroutine to reap processes
func StartReaper() {
	logrus.Infof("Starting reaper")
	go func() {
		sigs := make(chan os.Signal, 10)
		signal.Notify(sigs, syscall.SIGCHLD)
		for {
			// Wait for a child to terminate
			sig := <-sigs
			logrus.Infof("Signal received: %v", sig)
			for {
				// Reap processes
				cpid, _ := syscall.Wait4(-1, nil, syscall.WNOHANG, nil)
				if cpid < 1 {
					break
				}

				logrus.Infof("Reaped process with pid %d", cpid)
			}
		}
	}()
}
