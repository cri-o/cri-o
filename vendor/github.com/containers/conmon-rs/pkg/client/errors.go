package client

import (
	"errors"
	"fmt"
	"strings"

	"capnproto.org/go/capnp/v3/exc"
)

var (
	// ErrUnsupported gets returned if the server does not support the
	// feature. Errors which match it also match [errors.ErrUnsupported], so
	// that callers which check for the latter detect it as well.
	ErrUnsupported error = unsupportedError{}

	// ErrMissingConfig gets returned if a function or method gets called
	// with a nil configuration.
	ErrMissingConfig = errors.New("config must be specified")

	// ErrInvalidValue gets matched by the error of New and of a method if
	// the configuration has a value which none of the constants of its type
	// names, like an unknown CgroupManager or LogDriverType, or lacks a
	// required one, like the ServerRunDir of ConmonServerConfig or the
	// NetNsPath of ServePortForwardContainerConfig. New only checks the
	// Runtime, LogLevel and LogDriver if it starts a server, they do not
	// apply to one which runs already. Use errors.Is to check for it.
	ErrInvalidValue = errors.New("invalid value")

	// ErrContainerNotFound gets matched by the error of a method if the
	// server does not know the container of the request, for example because
	// it got created by another server or the server restarted. Use
	// errors.Is to check for it, the message of the error stays the one of
	// the server. ExecSyncContainer does not look the container up, the
	// runtime runs the command, the result of one which the runtime does not
	// know has the exit code ExecExitCodeNotStarted.
	ErrContainerNotFound = errors.New("container not found")

	// ErrTooManyRequests gets matched by the error of ServeExecContainer,
	// ServeAttachContainer and ServePortForwardContainer if too many URLs of
	// the kind wait for their client already. Use errors.Is to check for it,
	// the message of the error stays the one of the server. A later request
	// succeeds once a client took its URL or a URL expired. Older servers
	// drop the oldest URL instead.
	ErrTooManyRequests = errors.New("maximum number of in-flight requests exceeded")

	// ErrClientClosed gets returned by the methods of a client after Close.
	ErrClientClosed = errors.New("client is closed")

	// ErrShutdownTimeout gets returned by Shutdown and Kill if the server is
	// still running once the wait for it timed out, while an ended context
	// fails with the error of the context. After Shutdown, Kill sends
	// SIGKILL, which a server only survives while it is stuck in the kernel,
	// Kill returns ErrShutdownTimeout again then.
	ErrShutdownTimeout = errors.New("timed out waiting for server PID to disappear")

	// ErrServerExited gets matched by the error of New if the server it
	// started exited after it forked into the background but before it
	// accepted connections, for example because it crashed. Only a server
	// with a LogDriver other than LogDriverNone has logs which tell why,
	// the forked server writes no output otherwise, its stdio is /dev/null.
	// A crash aborts the server and leaves a core dump, which coredumpctl
	// lists on systemd hosts. A server which fails before it forks, like on
	// an invalid configuration, makes New fail with the error of the launch
	// instead. That error carries what the server wrote to stderr, unless
	// the LogDriver is stdout, whose Stderr gets it then.
	ErrServerExited = errors.New("server exited before accepting connections")

	// ErrServerNotUp gets matched by the error of New if the server it
	// started still runs but did not accept connections in time, or the
	// context ended before.
	ErrServerNotUp = errors.New("server did not accept connections")

	// ErrServerPIDUnknown gets matched by the error of Shutdown and Kill if
	// the client does not know the PID of the server, which they do not
	// signal then, for example because the server did not report it.
	ErrServerPIDUnknown = errors.New("server PID unknown")

	// ErrServerDisconnected gets matched by the error of a method if the
	// connection to the server ended before the request could be sent or
	// during the call, for example because the server exited, unlike a
	// request which the server rejected. The error does not tell whether
	// the server got the request. Calls in flight while Close, Shutdown or
	// Kill reset the connection match it as well, or fail with
	// ErrClientClosed. The next call connects again, also to a server which
	// replaced the exited one, after Close it fails with ErrClientClosed
	// instead. Use errors.Is to check for it.
	ErrServerDisconnected = errors.New("disconnected from the server")

	// ErrServerUnreachable gets matched by the error of a method if no
	// server listens on the socket of the ServerRunDir when the client
	// connects, for example because the server exited: the socket does not
	// exist, refuses the connection or is no stream socket. The next call
	// connects again, also to a server which got started in the meantime.
	// Use errors.Is to check for it.
	ErrServerUnreachable = errors.New("no server listens on the socket")

	// ErrServerNotResponding is returned by New if a server accepts
	// connections on the socket of the run directory but does not answer in
	// time, or the backlog of the socket is full. New does not start another
	// server then, it would take over the sockets of the one which runs and
	// orphan its containers. The error of a method matches it as well if
	// the backlog is full when the client connects, the next call connects
	// again. Use errors.Is to check for it.
	ErrServerNotResponding = errors.New("server accepted the connection but did not respond")
)

// unsupportedError is the type of ErrUnsupported. It matches
// errors.ErrUnsupported without wrapping it, which would change the message.
type unsupportedError struct{}

func (unsupportedError) Error() string {
	return "feature not supported by this conmon-rs version"
}

// Is matches errors.ErrUnsupported, errors.Is matches ErrUnsupported itself
// by comparing it.
func (unsupportedError) Is(target error) bool {
	return target == errors.ErrUnsupported
}

// containerNotFoundMessages are how servers start the message for a container
// they do not know: since the message got stable, and before.
var containerNotFoundMessages = []string{ErrContainerNotFound.Error(), "child not available"}

// requestLostWithConn reports whether a call failed because its request
// could not be sent on a connection which ended: its write failed, like with
// EPIPE right after the server exited, before the read of the connection
// noticed that, or the RPC system dropped the request while it shut the
// connection down. It reports both as a failed call which wraps the reason,
// not as a disconnected one. A failure of the server only carries a message.
func requestLostWithConn(exception *exc.Exception) bool {
	if connClosedOnWrite(exception) {
		return true
	}

	for inner := exception; errors.As(inner.Cause, &inner); {
		if inner.Type == exc.Disconnected {
			return true
		}
	}

	return false
}

// matchedError is the failure of a call which also matches an exported
// error, its message stays the one of the failure.
type matchedError struct {
	err    error
	target error
}

func (e *matchedError) Error() string {
	return e.err.Error()
}

func (e *matchedError) Unwrap() []error {
	return []error{e.err, e.target}
}

// rpcResult waits for the result of a call. Its failure matches the
// exported error for it, if any, see rpcError.
//
//nolint:ireturn // T is the result struct of the call, not an interface
func rpcResult[T any](future interface{ Struct() (T, error) }) (T, error) {
	result, err := future.Struct()
	if err != nil {
		return result, fmt.Errorf("get result: %w", rpcError(err))
	}

	return result, nil
}

// rpcError makes the failure of a call match the exported error for it, if
// any. The server reports every failure as an exception with a message, an
// ended connection fails calls with a disconnected one, or with a failed one
// for a request which could not be sent, see requestLostWithConn.
func rpcError(err error) error {
	var exception *exc.Exception
	if !errors.As(err, &exception) {
		return err
	}

	if exception.Type == exc.Disconnected || requestLostWithConn(exception) {
		return &matchedError{err: err, target: ErrServerDisconnected}
	}

	if exception.Type != exc.Failed {
		return err
	}

	// The message of the server, without the method the client put in
	// front. Only its start counts, the output of the runtime in other
	// failures may contain anything.
	message := exception.Cause
	for inner := errors.Unwrap(message); inner != nil; inner = errors.Unwrap(inner) {
		message = inner
	}

	for _, notFound := range containerNotFoundMessages {
		if strings.HasPrefix(message.Error(), notFound) {
			return &matchedError{err: err, target: ErrContainerNotFound}
		}
	}

	if strings.HasPrefix(message.Error(), ErrTooManyRequests.Error()) {
		return &matchedError{err: err, target: ErrTooManyRequests}
	}

	return err
}
