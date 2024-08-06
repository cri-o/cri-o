// Package exc provides an error type for capnp exceptions.
package exc

import (
	"errors"
	"strconv"
)

// Exception is an error that designates a Cap'n Proto exception.
type Exception struct {
	Type   Type
	Prefix string
	Cause  error
}

type wrappedError struct {
	prefix string
	base   error
}

func (e wrappedError) Error() string {
	return e.prefix + ": " + e.base.Error()
}

func (e wrappedError) Unwrap() error {
	return e.base
}

func WrapError(prefix string, err error) error {
	if err == nil {
		return nil
	}
	return wrappedError{
		prefix: prefix,
		base:   err,
	}
}

// New creates a new error that formats as "<prefix>: <msg>".
// The type can be recovered using the TypeOf() function.
func New(typ Type, prefix, msg string) *Exception {
	return &Exception{typ, prefix, errors.New(msg)}
}

func (e Exception) Error() string {
	if e.Prefix == "" {
		return e.Cause.Error()
	}

	return WrapError(e.Prefix, e.Cause).Error()
}

func (e Exception) Unwrap() error { return e.Cause }

func (e Exception) GoString() string {
	return "errors.Error{Type: " +
		e.Type.GoString() +
		", Prefix: " +
		strconv.Quote(e.Prefix) +
		", Cause: " +
		strconv.Quote(e.Cause.Error()) +
		"}"
}

// Annotate is creates a new error that formats as "<prefix>: <msg>: <e>".
// If e.Prefix == prefix, the prefix will not be duplicated.
// The returned Error.Type == e.Type.
func (e Exception) Annotate(prefix, msg string) *Exception {
	if prefix != e.Prefix {
		return &Exception{e.Type, prefix, WrapError(msg, e)}
	}

	return &Exception{e.Type, prefix, WrapError(msg, e.Cause)}
}

// Annotate creates a new error that formats as "<prefix>: <msg>: <err>".
// If err has the same prefix, then the prefix won't be duplicated.
// The returned error's type will match err's type.
func Annotate(prefix, msg string, err error) *Exception {
	if err == nil {
		return nil
	}

	if ce, ok := err.(*Exception); ok {
		return ce.Annotate(prefix, msg)
	}

	return &Exception{
		Type:   Failed,
		Prefix: prefix,
		Cause:  WrapError(msg, err),
	}
}

type Annotator string

func (f Annotator) New(t Type, err error) *Exception {
	if err == nil {
		return nil
	}

	return &Exception{
		Type:   t,
		Prefix: string(f),
		Cause:  err,
	}
}

func (f Annotator) Failed(err error) *Exception {
	return f.New(Failed, err)
}

func (f Annotator) WrapFailed(msg string, err error) *Exception {
	return f.New(Failed, WrapError(msg, err))
}

func (f Annotator) Disconnected(err error) *Exception {
	return f.New(Disconnected, err)
}

func (f Annotator) WrapDisconnected(msg string, err error) *Exception {
	return f.New(Disconnected, WrapError(msg, err))
}

func (f Annotator) Unimplemented(err error) *Exception {
	return f.New(Unimplemented, err)
}

func (f Annotator) WrapUnimplemented(msg string, err error) *Exception {
	return f.New(Unimplemented, WrapError(msg, err))
}

func (f Annotator) Annotate(err error, msg string) *Exception {
	return Annotate(string(f), msg, err)
}
