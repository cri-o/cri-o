package oci

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/containernetworking/plugins/pkg/ns"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"

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

		debug := func(format string, args ...any) {
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

func (r *runtimeOCI) StartWatchContainerMonitor(ctx context.Context, c *Container) error {
	if c.state.ContainerMonitorProcess == nil {
		log.Debugf(ctx, "Skipping start watch container monitor for container %q: conmon process is nil. maybe the container have existed before cri-o update", c.ID())

		return nil
	}

	// Check if the conmon process is the same process when the container was created.
	conmonPid := c.state.ContainerMonitorProcess.Pid

	pidfd, err := unix.PidfdOpen(conmonPid, 0)
	if err != nil {
		return fmt.Errorf("failed to open pidfd: %w", err)
	}

	startTime, err := getPidStartTime(conmonPid)
	if err != nil {
		return fmt.Errorf("failed to get conmon process start time: %w", err)
	}

	if c.state.ContainerMonitorProcess.StartTime != startTime {
		return errors.New("conmon process has gone because the start time changed")
	}

	return r.processMonitor.AddProcess(c, pidfd, r.handleConmonExit)
}

// handleConmonExit handles when the monitor process exits.
// When the container is killed, conmon generates an exit file for
// the container, and CRI-O handles exit files as follows:
//
// 1. Catch event of the exit file creation
// 2. Change the container state (the state is blocked while updating)
// 3. Remove the exit file
//
// So when the conmon stopped, the exit file must exist OR the container
// must be stopped, otherwise we can assume the conmon stopped unexpectedly.
// Just checking the status is not enough because when CRI-O is at step 1,
// the container is running, but the conmon might be stopped.
func (r *runtimeOCI) handleConmonExit(ctx context.Context, container *Container) {
	_, err := os.Stat(filepath.Join(r.config.ContainerExitsDir, container.ID()))
	if errors.Is(err, os.ErrNotExist) {
		if container.State().Status != rspec.StateStopped {
			log.Errorf(ctx, "Monitor for container %q stopped though the container is not stopped", container.ID())
			// TODO(bitoku): should we stop the container?
		}
	} else if err != nil {
		log.Errorf(ctx, "Failed to stat exit file for container %q: %v", container.ID(), err)
	}
}
