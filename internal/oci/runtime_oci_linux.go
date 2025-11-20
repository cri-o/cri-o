//go:build linux

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
// If reverse is true, it creates a listener in the container's network namespace
// and forwards connections from the container to the host stream.
// Otherwise, it connects to the container's port and forwards to the stream (normal mode).
func (r *runtimeOCI) PortForwardContainer(ctx context.Context, c *Container, netNsPath string, port int32, stream io.ReadWriteCloser, reverse bool) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	modeStr := "forward"
	if reverse {
		modeStr = "reverse"
	}

	log.Infof(ctx,
		"Starting %s port forward for %s (port %d) in network namespace %s",
		modeStr, c.ID(), port, netNsPath,
	)

	if reverse {
		return r.reversePortForward(ctx, c, netNsPath, port, stream)
	}

	return r.forwardPortForward(ctx, c, netNsPath, port, stream)
}

// forwardPortForward implements traditional port forwarding (host -> container).
func (r *runtimeOCI) forwardPortForward(ctx context.Context, c *Container, netNsPath string, port int32, stream io.ReadWriteCloser) error {
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

		return r.bidirectionalCopy(ctx, c, port, stream, conn, false)
	}); err != nil {
		return fmt.Errorf(
			"port forward into network namespace %q: %w", netNsPath, err,
		)
	}

	log.Infof(ctx, "Finished port forwarding for %q on port %d", c.ID(), port)

	return nil
}

// reversePortForward implements reverse port forwarding (container -> host).
// It creates a listener in the container's network namespace and forwards
// incoming connections from the container to the host stream.
func (r *runtimeOCI) reversePortForward(ctx context.Context, c *Container, netNsPath string, port int32, stream io.ReadWriteCloser) error {
	if err := ns.WithNetNSPath(netNsPath, func(_ ns.NetNS) error {
		defer stream.Close()

		// Create a listener in the container's network namespace
		listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
		if err != nil {
			return fmt.Errorf("failed to listen on localhost:%d inside namespace %s: %w", port, c.ID(), err)
		}
		defer listener.Close()

		log.Infof(ctx, "Reverse port forward listening on localhost:%d in container %s", port, c.ID())

		// Accept a single connection from the container
		// Use a channel to handle both connection acceptance and context cancellation
		connCh := make(chan net.Conn, 1)
		errCh := make(chan error, 1)

		go func() {
			conn, err := listener.Accept()
			if err != nil {
				errCh <- err
				return
			}
			connCh <- conn
		}()

		// Wait for connection or context cancellation
		var conn net.Conn
		select {
		case conn = <-connCh:
			// Connection accepted
			defer conn.Close()
		case err := <-errCh:
			return fmt.Errorf("failed to accept connection on localhost:%d inside namespace %s: %w", port, c.ID(), err)
		case <-ctx.Done():
			log.Debugf(ctx, "Reverse port forward cancelled before connection: %v", ctx.Err())
			return ctx.Err()
		}

		log.Debugf(ctx, "Reverse port forward accepted connection from container on port %d", port)

		return r.bidirectionalCopy(ctx, c, port, stream, conn, true)
	}); err != nil {
		return fmt.Errorf(
			"reverse port forward into network namespace %q: %w", netNsPath, err,
		)
	}

	log.Infof(ctx, "Finished reverse port forwarding for %q on port %d", c.ID(), port)

	return nil
}

// bidirectionalCopy copies data bidirectionally between the stream and conn.
// It handles graceful shutdown and timeouts.
func (r *runtimeOCI) bidirectionalCopy(ctx context.Context, c *Container, port int32, stream io.ReadWriteCloser, conn net.Conn, reverse bool) error {
	errCh := make(chan error, 2)

	debug := func(format string, args ...any) {
		mode := "forward"
		if reverse {
			mode = "reverse"
		}
		log.Debugf(ctx, fmt.Sprintf(
			"PortForward[%s] (id: %s, port: %d): %s", mode, c.ID(), port, format,
		), args...)
	}

	// Copy from the connection to the stream
	go func() {
		debug("copy data from container to client")
		_, err := io.Copy(stream, conn)
		errCh <- err
	}()

	// Copy from the stream to the connection
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
}
