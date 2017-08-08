package errors

import (
	"errors"
)

var (
	// ErrInvalidArg is used for invalid user-provided data
	ErrInvalidArg = errors.New("invalid argument")
	// ErrNotSupported is used when a feature not supported on the current
	// system is requested
	ErrNotSupported = errors.New("this feature is not supported")
	// ErrNoSuchSandbox is used to indicate the operation targets a sandbox
	// that does not exist
	ErrNoSuchSandbox = errors.New("no such sandbox")
	// ErrNoSuchContainer is used to indicate the operation targets a
	// container that does not exist
	ErrNoSuchContainer = errors.New("no such container")
	// ErrInternal is used when an internal library error is encountered
	ErrInternal = errors.New("internal library error")
)
