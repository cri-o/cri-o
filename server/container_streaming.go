package server

import (
	"context"
	"errors"
	"fmt"

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
func monitorStreamingURL(
	ctx context.Context,
	enabled func(runtimeHandler string) (bool, error),
	runtimeHandler, request, target string,
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

	if !errors.Is(err, errors.ErrUnsupported) {
		return "", streamingError(err, fmt.Sprintf("could not serve %s for %s", request, target))
	}

	log.Infof(
		ctx,
		"Container monitor of %s does not support %s, using the CRI-O streaming server: %v",
		target, request, err,
	)

	return "", nil
}

// streamingError returns the error of the container monitor for a streaming
// request with the message in front. If too many URLs wait for their client
// already, it gets ResourceExhausted, like the in-flight limit of CRI-O's
// streaming server.
func streamingError(err error, message string) error {
	if errors.Is(err, conmonClient.ErrTooManyRequests) {
		return status.Errorf(codes.ResourceExhausted, "%s: %v", message, err)
	}

	return fmt.Errorf("%s: %w", message, err)
}
