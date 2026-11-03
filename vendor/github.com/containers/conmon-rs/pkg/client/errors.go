package client

import (
	"errors"
	"fmt"
	"strings"

	"capnproto.org/go/capnp/v3/exc"
)

var (
	// ErrMissingIDMappings gets returned if user namespace unsharing is selected
	// but no IDMappings being provided.
	ErrMissingIDMappings = errors.New(
		"unsharing user namespace selected but no IDMappings provided",
	)

	// ErrUnsupported gets returned if the server does not support the
	// feature. Errors which match it also match [errors.ErrUnsupported], so
	// that callers which check for the latter detect it as well.
	ErrUnsupported error = unsupportedError{}

	// ErrCgroupManagerPerCommand gets returned if a per command configuration
	// asks for CgroupManagerPerCommand itself, which the server knows nothing
	// about.
	ErrCgroupManagerPerCommand = errors.New(
		"the per command cgroup manager cannot be CgroupManagerPerCommand itself",
	)

	// ErrMissingNetNsPath gets returned if port forwarding is requested
	// without the path of the network namespace.
	ErrMissingNetNsPath = errors.New("network namespace path is empty")

	// ErrMissingConfig gets returned if a function or method gets called
	// with a nil configuration.
	ErrMissingConfig = errors.New("config must be specified")

	// ErrInvalidValue gets matched by the error of New and of a method if
	// the configuration has a value which none of the constants of its type
	// names, like an unknown CgroupManager, LogDriverType or Namespace, and
	// if the server answered with a value the client does not know. New only
	// checks the LogLevel and LogDriver if it starts a server, they do not
	// apply to one which runs already. Use errors.Is to check for it.
	ErrInvalidValue = errors.New("invalid value")

	// ErrTerminalSizeNil gets returned by SetWindowSizeContainer if the
	// configuration has no Size.
	ErrTerminalSizeNil = errors.New("terminal size cannot be nil")

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

	// ErrShutdownTimeout gets returned by ShutdownContext (or Shutdown) and
	// KillContext (or Kill) if the server is still running once the wait for
	// it timed out, while an ended context fails with the error of the
	// context. After ShutdownContext, KillContext sends SIGKILL, which a
	// server only survives while it is stuck in the kernel, KillContext
	// returns ErrShutdownTimeout again then.
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

	// ErrServerPIDUnknown gets matched by the error of Shutdown and
	// KillContext if the client does not know the PID of the server, which
	// they do not signal then, for example because the server did not report
	// it.
	ErrServerPIDUnknown = errors.New("server PID unknown")

	// ErrServerDisconnected gets matched by the error of a method if the
	// connection to the server ended during the call, for example because the
	// server exited, unlike a request which the server rejected. Calls in
	// flight while Close, Shutdown or KillContext reset the connection match
	// it as well, or fail with ErrClientClosed. The next call connects again,
	// also to a server which replaced the exited one, after Close it fails
	// with ErrClientClosed instead. Use errors.Is to check for it.
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
	// orphan its containers.
	ErrServerNotResponding = errors.New("server accepted the connection but did not respond")

	// ErrRemoteFDsBroken gets matched by the error of a request of
	// RemoteFDs which got no valid response, which got interrupted by its
	// context after it was sent, or which ran into Close. Every request
	// afterwards fails with it as well, a new RemoteFDs is needed then. A
	// request which the server rejected, or which could not be sent, does
	// not match it and keeps the connection usable.
	ErrRemoteFDsBroken = errors.New("fd socket connection broken")

	// ErrTooManyFileDescriptors gets returned by Send and SendContext of
	// RemoteFDs for more than 253 file descriptors, which the kernel does
	// not pass in one message. Nothing gets sent then, the connection stays
	// usable.
	ErrTooManyFileDescriptors = errors.New("too many file descriptors")
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

// matchedError is a failure the server reported, which also matches an
// exported error.
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
// ended connection fails calls with a disconnected one.
func rpcError(err error) error {
	var exception *exc.Exception
	if !errors.As(err, &exception) {
		return err
	}

	if exception.Type == exc.Disconnected {
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
