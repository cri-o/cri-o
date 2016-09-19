package hcsshim

import (
	"errors"
	"fmt"
	"syscall"
)

var (
	// ErrComputeSystemDoesNotExist is an error encountered when the container being operated on no longer exists
	ErrComputeSystemDoesNotExist = syscall.Errno(0xc037010e)

	// ErrElementNotFound is an error encountered when the object being referenced does not exist
	ErrElementNotFound = syscall.Errno(0x490)

	// ErrHandleClose is an error encountered when the handle generating the notification being waited on has been closed
	ErrHandleClose = errors.New("hcsshim: the handle generating this notification has been closed")

	// ErrInvalidNotificationType is an error encountered when an invalid notification type is used
	ErrInvalidNotificationType = errors.New("hcsshim: invalid notification type")

	// ErrInvalidProcessState is an error encountered when the process is not in a valid state for the requested operation
	ErrInvalidProcessState = errors.New("the process is in an invalid state for the attempted operation")

	// ErrTimeout is an error encountered when waiting on a notification times out
	ErrTimeout = errors.New("hcsshim: timeout waiting for notification")

	// ErrUnexpectedContainerExit is the error encountered when a container exits while waiting for
	// a different expected notification
	ErrUnexpectedContainerExit = errors.New("unexpected container exit")

	// ErrUnexpectedProcessAbort is the error encountered when communication with the compute service
	// is lost while waiting for a notification
	ErrUnexpectedProcessAbort = errors.New("lost communication with compute service")

	// ErrUnexpectedValue is an error encountered when hcs returns an invalid value
	ErrUnexpectedValue = errors.New("unexpected value returned from hcs")

	// ErrVmcomputeAlreadyStopped is an error encountered when a shutdown or terminate request is made on a stopped container
	ErrVmcomputeAlreadyStopped = syscall.Errno(0xc0370110)

	// ErrVmcomputeOperationPending is an error encountered when the operation is being completed asynchronously
	ErrVmcomputeOperationPending = syscall.Errno(0xC0370103)

	// ErrVmcomputeOperationInvalidState is an error encountered when the compute system is not in a valid state for the requested operation
	ErrVmcomputeOperationInvalidState = syscall.Errno(0xc0370105)

	// ErrProcNotFound is an error encountered when the the process cannot be found
	ErrProcNotFound = syscall.Errno(0x7f)
)

// ProcessError is an error encountered in HCS during an operation on a Process object
type ProcessError struct {
	Process   *process
	Operation string
	ExtraInfo string
	Err       error
}

// ContainerError is an error encountered in HCS during an operation on a Container object
type ContainerError struct {
	Container *container
	Operation string
	ExtraInfo string
	Err       error
}

func (e *ContainerError) Error() string {
	if e == nil {
		return "<nil>"
	}

	if e.Container == nil {
		return "unexpected nil container for error: " + e.Err.Error()
	}

	s := "container " + e.Container.id

	if e.Operation != "" {
		s += " encountered an error during " + e.Operation
	}

	if e.Err != nil {
		s += fmt.Sprintf(" failed in Win32: %s (0x%x)", e.Err, win32FromError(e.Err))
	}

	if e.ExtraInfo != "" {
		s += " extra info: " + e.ExtraInfo
	}

	return s
}

func makeContainerError(container *container, operation string, extraInfo string, err error) error {
	// Don't double wrap errors
	if _, ok := err.(*ContainerError); ok {
		return err
	}
	containerError := &ContainerError{Container: container, Operation: operation, ExtraInfo: extraInfo, Err: err}
	return containerError
}

func (e *ProcessError) Error() string {
	if e == nil {
		return "<nil>"
	}

	if e.Process == nil {
		return "Unexpected nil process for error: " + e.Err.Error()
	}

	s := fmt.Sprintf("process %d", e.Process.processID)

	if e.Process.container != nil {
		s += " in container " + e.Process.container.id
	}

	if e.Operation != "" {
		s += " " + e.Operation
	}

	switch e.Err.(type) {
	case nil:
		break
	case syscall.Errno:
		s += fmt.Sprintf(" failed in Win32: %s (0x%x)", e.Err, win32FromError(e.Err))
	default:
		s += fmt.Sprintf(" failed: %s", e.Error())
	}

	return s
}

func makeProcessError(process *process, operation string, extraInfo string, err error) error {
	// Don't double wrap errors
	if _, ok := err.(*ProcessError); ok {
		return err
	}
	processError := &ProcessError{Process: process, Operation: operation, ExtraInfo: extraInfo, Err: err}
	return processError
}

// IsNotExist checks if an error is caused by the Container or Process not existing.
// Note: Currently, ErrElementNotFound can mean that a Process has either
// already exited, or does not exist. Both IsAlreadyStopped and IsNotExist
// will currently return true when the error is ErrElementNotFound or ErrProcNotFound.
func IsNotExist(err error) bool {
	err = getInnerError(err)
	return err == ErrComputeSystemDoesNotExist ||
		err == ErrElementNotFound ||
		err == ErrProcNotFound
}

// IsPending returns a boolean indicating whether the error is that
// the requested operation is being completed in the background.
func IsPending(err error) bool {
	err = getInnerError(err)
	return err == ErrVmcomputeOperationPending
}

// IsTimeout returns a boolean indicating whether the error is caused by
// a timeout waiting for the operation to complete.
func IsTimeout(err error) bool {
	err = getInnerError(err)
	return err == ErrTimeout
}

// IsAlreadyStopped returns a boolean indicating whether the error is caused by
// a Container or Process being already stopped.
// Note: Currently, ErrElementNotFound can mean that a Process has either
// already exited, or does not exist. Both IsAlreadyStopped and IsNotExist
// will currently return true when the error is ErrElementNotFound or ErrProcNotFound.
func IsAlreadyStopped(err error) bool {
	err = getInnerError(err)
	return err == ErrVmcomputeAlreadyStopped ||
		err == ErrElementNotFound ||
		err == ErrProcNotFound
}

func getInnerError(err error) error {
	switch pe := err.(type) {
	case nil:
		return nil
	case *ContainerError:
		err = pe.Err
	case *ProcessError:
		err = pe.Err
	}
	return err
}
