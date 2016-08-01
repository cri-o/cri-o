package utils

import (
	"bytes"
	"os/exec"
	"syscall"
)

const PR_SET_CHILD_SUBREAPER = 36

func ExecCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return out.String(), nil
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
