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

	// ErrUnsupported gets returned if the server does not support the feature.
	ErrUnsupported = errors.New("feature not supported by this conmon-rs version")

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

	// ErrShutdownTimeout gets returned by Shutdown if the server is still
	// running once the wait for it timed out, while an ended context fails
	// with the error of the context. KillContext stops it for good.
	ErrShutdownTimeout = errors.New("timed out waiting for server PID to disappear")

	// ErrServerExited gets matched by the error of New if the server it
	// started exited before it accepted connections, for example for an
	// invalid configuration. Its logs tell why.
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
)

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
