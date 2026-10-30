package server

import (
	"context"
	"errors"
	"fmt"
	"syscall"

	conmonClient "github.com/containers/conmon-rs/pkg/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cri-o/cri-o/internal/log"
)

// monitorStreamingURL returns the URL of the container monitor for an exec,
// attach or port forward request of target, or an empty one if CRI-O's
// streaming server serves the request: if the runtime handler does not
// enable it for the monitor, or if serve fails with errors.ErrUnsupported. A
// conmon-rs server which got started by an older conmon-rs keeps running
// until its pod stops, and the infra container of a pod may resolve to
// another runtime type.
//
// CRI-O's streaming server also serves the request if the container monitor
// is unreachable or does not know the container, unless needsMonitor is set.
// The conmon-rs server runs in the cgroup of the pod and may get OOM killed,
// and if it is gone when CRI-O restarts, CRI-O starts a new one which does
// not know the containers of the pod. CRI-O execs into the container and
// forwards ports without the monitor, but attaches through it, so that an
// attach request fails then.
func monitorStreamingURL(
	ctx context.Context,
	enabled func(runtimeHandler string) (bool, error),
	runtimeHandler, request, target string,
	needsMonitor bool,
	serve func() (string, error),
) (string, error) {
	if ok, err := enabled(runtimeHandler); err != nil || !ok {
		return "", nil
	}

	log.Debugf(
		ctx,
		"Runtime handler %q serves %s from the container monitor",
		runtimeHandler,
		request,
	)

	url, err := serve()
	if err == nil {
		log.Infof(ctx, "Using %s URL from container monitor", request)

		return url, nil
	}

	if errors.Is(err, errors.ErrUnsupported) {
		log.Infof(
			ctx,
			"Container monitor of %s does not support %s, using the CRI-O streaming server: %v",
			target, request, err,
		)

		return "", nil
	}

	if reason := monitorUnavailableReason(err); reason != "" && !needsMonitor {
		log.Warnf(
			ctx,
			"Container monitor of %s %s, using the CRI-O streaming server for %s: %v",
			target, reason, request, err,
		)

		return "", nil
	}

	return "", streamingError(err, fmt.Sprintf("could not serve %s for %s", request, target))
}

// monitorUnavailableReason returns why the container monitor cannot serve a
// streaming request according to its error, or an empty string if the error
// does not tell that the monitor is unreachable or does not know the
// container.
func monitorUnavailableReason(err error) string {
	switch {
	case monitorUnreachable(err):
		return "is unreachable"
	case errors.Is(err, conmonClient.ErrContainerNotFound):
		return "does not know the container"
	default:
		return ""
	}
}

// monitorUnreachable returns whether the error of the container monitor
// tells that its server is gone: dialing its socket failed because no server
// listens on it anymore, or the connection ended during the call, for
// example because the server exited.
func monitorUnreachable(err error) bool {
	return errors.Is(err, conmonClient.ErrServerDisconnected) ||
		errors.Is(err, syscall.ENOENT) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPROTOTYPE)
}

// streamingError returns the error of the container monitor for a streaming
// request with the message in front. If too many URLs wait for their client
// already, it gets ResourceExhausted, like the in-flight limit of CRI-O's
// streaming server. If the monitor is unreachable, it gets Unavailable, and
// NotFound if the monitor does not know the container.
func streamingError(err error, message string) error {
	switch {
	case errors.Is(err, conmonClient.ErrTooManyRequests):
		return status.Errorf(codes.ResourceExhausted, "%s: %v", message, err)
	case monitorUnreachable(err):
		return status.Errorf(
			codes.Unavailable,
			"%s: container monitor is unreachable: %v",
			message,
			err,
		)
	case errors.Is(err, conmonClient.ErrContainerNotFound):
		return status.Errorf(codes.NotFound, "%s: %v", message, err)
	default:
		return fmt.Errorf("%s: %w", message, err)
	}
}
