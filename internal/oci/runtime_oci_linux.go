package oci

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/containernetworking/plugins/pkg/ns"

	"github.com/cri-o/cri-o/internal/log"
)

// PortForwardContainer forwards the specified port into the provided container.
func (r *runtimeOCI) PortForwardContainer(ctx context.Context, c *Container, netNsPath string, port int32, stream io.ReadWriteCloser) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	log.Infof(ctx,
		"Starting port forward for %s in network namespace %s", c.ID(), netNsPath,
	)

	// Adapted reference implementation:
	// https://github.com/containerd/cri/blob/8c366d/pkg/server/sandbox_portforward_unix.go#L65-L120
	if err := ns.WithNetNSPath(netNsPath, func(_ ns.NetNS) error {
		defer stream.Close()

		// localhost can resolve to both IPv4 and IPv6 addresses in dual-stack systems
		// but the application can be listening in one of the IP families only.
		// golang has enabled RFC 6555 Fast Fallback (aka HappyEyeballs) by default in 1.12
		// It means that if a host resolves to both IPv6 and IPv4, it will try to connect to any
		// of those addresses and use the working connection.
		// xref https://github.com/golang/go/commit/efc185029bf770894defe63cec2c72a4c84b2ee9
		// However, the implementation uses go routines to start both connections in parallel,
		// and this has limitations when running inside a namespace, so we try to the connections
		// serially disabling the Fast Fallback support.
		// xref https://github.com/golang/go/issues/44922
		var d net.Dialer
		d.FallbackDelay = -1
		conn, err := d.Dial("tcp", fmt.Sprintf("localhost:%d", port))
		if err != nil {
			return fmt.Errorf("failed to connect to localhost:%d inside namespace %s: %w", port, c.ID(), err)
		}
		defer conn.Close()

		errCh := make(chan error, 2)

		debug := func(format string, args ...interface{}) {
			log.Debugf(ctx, fmt.Sprintf(
				"PortForward (id: %s, port: %d): %s", c.ID(), port, format,
			), args...)
		}

		// Copy from the namespace port connection to the client stream
		go func() {
			debug("copy data from container to client")
			_, err := io.Copy(stream, conn)
			errCh <- err
		}()

		// Copy from the client stream to the namespace port connection
		go func() {
			debug("copy data from client to container")
			_, err := io.Copy(conn, stream)
			errCh <- err
		}()

		// Wait until the first error is returned by one of the connections we
		// use errFwd to store the result of the port forwarding operation if
		// the context is cancelled close everything and return
		var errFwd error
		select {
		case errFwd = <-errCh:
			debug("stop forwarding in direction: %v", errFwd)
		case <-ctx.Done():
			debug("cancelled: %v", ctx.Err())
			return ctx.Err()
		}

		// give a chance to terminate gracefully or timeout
		const timeout = time.Second
		select {
		case e := <-errCh:
			if errFwd == nil {
				errFwd = e
			}
			debug("stopped forwarding in both directions")

		case <-time.After(timeout):
			debug("timed out waiting to close the connection")

		case <-ctx.Done():
			debug("cancelled: %v", ctx.Err())
			errFwd = ctx.Err()
		}

		return errFwd
	}); err != nil {
		return fmt.Errorf(
			"port forward into network namespace %q: %w", netNsPath, err,
		)
	}

	log.Infof(ctx, "Finished port forwarding for %q on port %d", c.ID(), port)
	return nil
}
