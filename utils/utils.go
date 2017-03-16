package utils

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"

	systemdDbus "github.com/coreos/go-systemd/dbus"
	"github.com/godbus/dbus"
)

// ExecCmd executes a command with args and returns its output as a string along
// with an error, if any
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
func ExecCmdWithStdStreams(stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error {
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

// Prctl is a way to make the prctl linux syscall
func Prctl(option int, arg2, arg3, arg4, arg5 uintptr) (err error) {
	_, _, e1 := syscall.Syscall6(syscall.SYS_PRCTL, uintptr(option), arg2, arg3, arg4, arg5, 0)
	if e1 != 0 {
		err = e1
	}
	return
}

// StatusToExitCode converts wait status code to an exit code
func StatusToExitCode(status int) int {
	return ((status) & 0xff00) >> 8
}

// RunUnderSystemdScope adds the specified pid to a systemd scope
func RunUnderSystemdScope(pid int, slice string, unitName string) error {
	var properties []systemdDbus.Property
	conn, err := systemdDbus.New()
	if err != nil {
		return err
	}
	properties = append(properties, systemdDbus.PropSlice(slice))
	properties = append(properties, newProp("PIDs", []uint32{uint32(pid)}))
	properties = append(properties, newProp("Delegate", true))
	properties = append(properties, newProp("DefaultDependencies", false))
	_, err = conn.StartTransientUnit(unitName, "replace", properties, nil)
	return err
}

func newProp(name string, units interface{}) systemdDbus.Property {
	return systemdDbus.Property{
		Name:  name,
		Value: dbus.MakeVariant(units),
	}
}
