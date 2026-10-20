package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/containers/conmon-rs/internal/proto"
	"go.podman.io/common/pkg/detach"
	"go.podman.io/common/pkg/resize"
)

const (
	attachPacketBufSize = 8192
	attachPipeDone      = 0
	attachPipeStdout    = 2
	attachPipeStderr    = 3

	// attachOutputStopTimeout bounds the wait for the output to stop once
	// the context of an attach is done.
	attachOutputStopTimeout = time.Second
)

var errTerminalSizeNil = errors.New("terminal size cannot be nil")

// AttachStreams are the stdio streams for the AttachConfig.
type AttachStreams struct {
	// Standard input stream, can be nil.
	Stdin *In

	// Standard output stream, can be nil.
	Stdout *Out

	// Standard error stream, can be nil.
	Stderr *Out
}

// In defines an input stream.
type In struct {
	// Wraps an io.ReadCloser
	io.ReadCloser
}

// Out defines an output stream.
type Out struct {
	// Wraps an io.WriteCloser
	io.WriteCloser
}

// AttachConfig is the configuration for running the Attach method.
type AttachConfig struct {
	// ID of the container.
	ID string

	// Path of the attach socket.
	SocketPath string

	// ExecSession ID, if this is an attach for an Exec.
	//
	// Deprecated: The server only logs it, attach sessions are per container.
	ExecSession string

	// Whether a terminal was setup for the command this is attaching to.
	Tty bool

	// Whether the container stdin should be closed once the attach stdin
	// reaches EOF (stdin once). Without a terminal, the output is still
	// forwarded until the container closes it. A terminal is never closed,
	// so the attach returns on stdin EOF.
	//
	// The server applies the value of the latest attach call to all
	// connections it accepts afterwards on the same socket, so all attaches
	// of a container should use the same value, for example derived from
	// its config.
	StopAfterStdinEOF bool

	// Whether the container supports stdin or not.
	ContainerStdin bool

	// Whether the output is passed through the caller's std streams, rather than
	// ones created for the attach session. Only the pre attach function runs
	// then.
	//
	// Deprecated: Nothing is attached with it, call PreAttachFunc directly.
	Passthrough bool

	// Channel of resize events. They are applied until the attach returns,
	// the channel is not read afterwards. Senders should stop or close it
	// once the attach is over.
	Resize <-chan resize.TerminalSize

	// The standard streams for this attach session.
	Streams AttachStreams

	// A closure to be run before the streams are attached.
	// This could be used to start a container.
	PreAttachFunc func() error

	// A closure to be run after the streams are attached.
	// This could be used to notify callers the streams have been attached.
	PostAttachFunc func() error

	// The keys that indicate the attach session should be detached.
	DetachKeys []byte
}

// attachReaderValue is the value of the attachReaders map.
type attachReaderValue struct {
	stdin *In
}

// storeAttachReader registers the stdin of an attach session and returns its
// identifier.
func (c *ConmonClient) storeAttachReader(value *attachReaderValue) uint64 {
	id := c.attachReaderID.Add(1)

	c.attachReaderMu.Lock()
	defer c.attachReaderMu.Unlock()

	c.attachReaders[id] = value

	return id
}

// AttachContainer can be used to attach to a running container.
//
// Once the context is done, the attach returns its error. The output
// streams are closed before, unless a write to them stays blocked for longer
// than a second, which then may still finish and close them afterwards.
func (c *ConmonClient) AttachContainer(ctx context.Context, cfg *AttachConfig) error {
	ctx, span := c.startSpan(ctx, "AttachContainer")
	if span != nil {
		defer span.End()
	}

	rpcClient, err := c.client(ctx)
	if err != nil {
		return fmt.Errorf("create RPC client: %w", err)
	}

	future, free := rpcClient.AttachContainer(ctx, func(p proto.Conmon_attachContainer_Params) error {
		req, err := p.NewRequest()
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		if err := c.setMetadata(ctx, req); err != nil {
			return err
		}

		if err := req.SetId(cfg.ID); err != nil {
			return fmt.Errorf("set ID: %w", err)
		}

		if err := req.SetSocketPath(cfg.SocketPath); err != nil {
			return fmt.Errorf("set socket path: %w", err)
		}

		req.SetStopAfterStdinEof(cfg.StopAfterStdinEOF)

		if cfg.ExecSession != "" {
			if err := req.SetExecSessionId(cfg.ExecSession); err != nil {
				return fmt.Errorf("set exec session ID: %w", err)
			}
		}

		return nil
	})
	defer free()

	result, err := future.Struct()
	if err != nil {
		return fmt.Errorf("get result: %w", err)
	}

	if _, err := result.Response(); err != nil {
		return fmt.Errorf("get response: %w", err)
	}

	if err := c.attach(ctx, cfg); err != nil {
		return fmt.Errorf("run attach: %w", err)
	}

	return nil
}

func (c *ConmonClient) attach(ctx context.Context, cfg *AttachConfig) (err error) {
	var conn *net.UnixConn

	if !cfg.Passthrough {
		c.logger.Debugf("Attaching to container %s", cfg.ID)

		// The resize events are applied until the attach returns.
		resizeDone := make(chan struct{})
		defer close(resizeDone)

		go c.forwardResizeEvents(ctx, cfg, resizeDone)

		conn, err = DialLongSocket("unixpacket", cfg.SocketPath)
		if err != nil {
			return fmt.Errorf("failed to connect to container's attach socket: %v: %w", cfg.SocketPath, err)
		}

		defer func() {
			if err := conn.Close(); err != nil {
				c.logger.Errorf("unable to close socket: %q", err)
			}
		}()
	}

	if cfg.PreAttachFunc != nil {
		if err := cfg.PreAttachFunc(); err != nil {
			return fmt.Errorf("run pre attach func: %w", err)
		}
	}

	if cfg.Passthrough {
		return nil
	}

	receiveStdoutError, stdinDone, id := c.setupStdioChannels(ctx, cfg, conn)

	if cfg.PostAttachFunc != nil {
		if err := cfg.PostAttachFunc(); err != nil {
			// Stop copying stdin and receiving the output, like an ended
			// context does.
			c.tryCloseAttachReaderForID(id)
			c.stopOutput(func() {
				if err := conn.CloseRead(); err != nil {
					c.logger.Errorf("Unable to close conn: %v", err)
				}
			}, receiveStdoutError)

			return fmt.Errorf("run post attach func: %w", err)
		}
	}

	if err := c.readStdio(ctx, cfg, conn, id, receiveStdoutError, stdinDone); err != nil {
		return fmt.Errorf("read stdio: %w", err)
	}

	return nil
}

// forwardResizeEvents applies the resize events of the config to the
// container until the channel gets closed or done is closed, whichever
// comes first.
func (c *ConmonClient) forwardResizeEvents(ctx context.Context, cfg *AttachConfig, done <-chan struct{}) {
	if cfg.Resize == nil {
		return
	}

	for {
		select {
		case <-done:
			return
		case size, ok := <-cfg.Resize:
			if !ok {
				return
			}

			if size.Height < 1 || size.Width < 1 {
				continue
			}

			c.logger.Debugf("Got a resize event: %+v", size)

			if err := c.SetWindowSizeContainer(ctx, &SetWindowSizeContainerConfig{
				ID:   cfg.ID,
				Size: &size,
			}); err != nil {
				c.logger.Debugf("Failed to write to control file to resize terminal: %v", err)
			}
		}
	}
}

func (c *ConmonClient) setupStdioChannels(
	ctx context.Context, cfg *AttachConfig, conn *net.UnixConn,
) (receiveStdoutError, stdinDone chan error, id uint64) {
	ctx, span := c.startSpan(ctx, "setupStdioChannels")
	if span != nil {
		defer span.End()
	}

	// Buffered, so that the goroutines can always finish, even if nobody
	// receives their result anymore.
	receiveStdoutError = make(chan error, 1)

	go func() {
		receiveStdoutError <- c.redirectResponseToOutputStreams(ctx, cfg, conn)
	}()

	stdinDone = make(chan error, 1)
	stdin := cfg.Streams.Stdin

	if stdin != nil {
		id = c.storeAttachReader(&attachReaderValue{stdin: stdin})

		go func() {
			_, err := detach.Copy(conn, stdin, cfg.DetachKeys)
			if !errors.Is(err, io.ErrClosedPipe) {
				stdinDone <- err
			}
		}()
	}

	return receiveStdoutError, stdinDone, id
}

func (c *ConmonClient) redirectResponseToOutputStreams(
	ctx context.Context, cfg *AttachConfig, conn io.Reader,
) (err error) {
	ctx, span := c.startSpan(ctx, "redirectResponseToOutputStreams")
	if span != nil {
		defer span.End()
	}

	buf := make([]byte, attachPacketBufSize+1) /* Sync with conmonrs ATTACH_PACKET_BUF_SIZE */

	defer func() {
		if cfg.Streams.Stdout != nil {
			cfg.Streams.Stdout.Close()
		}

		if cfg.Streams.Stderr != nil {
			cfg.Streams.Stderr.Close()
		}
	}()

	for {
		c.logger.Trace("Waiting to read from attach connection")

		nr, er := conn.Read(buf)
		c.logger.WithError(er).Tracef("Got %d bytes from attach connection", nr)

		if nr > 0 {
			cont, er := c.handlePacket(ctx, cfg, buf, nr)
			if er != nil {
				return er
			}

			if !cont {
				return nil
			}
		}

		if er == io.EOF {
			return nil
		}

		if errors.Is(er, syscall.ECONNRESET) {
			c.logger.WithError(er).Trace("Connection reset, retrying to read")

			continue
		}

		if er != nil {
			err = er

			break
		}
	}

	if err != nil {
		return fmt.Errorf("redirect response to output streams: %w", err)
	}

	return nil
}

func (c *ConmonClient) handlePacket(
	ctx context.Context, cfg *AttachConfig, buf []byte, nr int,
) (cont bool, err error) {
	_, span := c.startSpan(ctx, "handlePacket")
	if span != nil {
		defer span.End()
	}

	var dst io.Writer

	switch buf[0] {
	case attachPipeDone:
		c.logger.Trace("Received done packet")

		return false, nil

	case attachPipeStdout:
		if cfg.Streams.Stdout == nil {
			c.logger.Debug("stdout for packet is nil")

			return true, nil
		}

		dst = cfg.Streams.Stdout

		c.logger.Trace("Received stdout packet")

	case attachPipeStderr:
		if cfg.Streams.Stderr == nil {
			c.logger.Debug("stderr for packet is nil")

			return true, nil
		}

		dst = cfg.Streams.Stderr

		c.logger.Trace("Received stderr packet")

	default:
		c.logger.Infof("Received unexpected attach type %+d", buf[0])

		return true, nil
	}

	nw, ew := dst.Write(buf[1:nr])
	c.logger.WithError(ew).Tracef("Wrote %d bytes to destination", nw)

	if ew != nil {
		return false, fmt.Errorf("failed to write packet %w", ew)
	}

	if nr != nw+1 {
		return false, io.ErrShortWrite
	}

	return true, nil
}

func (c *ConmonClient) tryCloseAttachReaderForID(id uint64) {
	c.logger.Tracef("Closing attach reader for ID: %d", id)

	c.attachReaderMu.Lock()
	value, ok := c.attachReaders[id]
	delete(c.attachReaders, id)
	c.attachReaderMu.Unlock()

	if ok {
		c.closeAttachReader(value)
	}
}

// closeAttachReader closes the stdin of an attach session. The attach
// socket stays, the server keeps listening on it until the container exits
// and removes it then.
func (c *ConmonClient) closeAttachReader(value *attachReaderValue) {
	if err := value.stdin.Close(); err != nil {
		c.logger.WithError(err).Warn("Unable to close attach reader")
	}
}

func (c *ConmonClient) readStdio(
	ctx context.Context, cfg *AttachConfig, conn *net.UnixConn, id uint64, receiveStdoutError, stdinDone chan error,
) (err error) {
	_, span := c.startSpan(ctx, "readStdio")
	if span != nil {
		defer span.End()
	}

	c.logger.Trace("Read stdio on attach")

	// Stopping the output can happen on stdin EOF and once more if the
	// context ends while waiting for the output.
	closeRead := sync.OnceFunc(func() {
		if err := conn.CloseRead(); err != nil {
			c.logger.Errorf("Unable to close conn: %v", err)
		}
	})

	select {
	case err = <-receiveStdoutError:
		c.logger.WithError(err).Trace("Received message on output channel")
		c.tryCloseAttachReaderForID(id)

		if closeErr := conn.CloseWrite(); closeErr != nil {
			return errors.Join(closeErr, err)
		}

		if err != nil {
			return fmt.Errorf("got stdout error: %w", err)
		}

		return nil

	case err = <-stdinDone:
		c.logger.WithError(err).Trace("Received message on input channel")

		c.tryCloseAttachReaderForID(id)

		// Copying stdin is done, close it. With StopAfterStdinEOF the server
		// closes the container stdin in turn, so the output written
		// afterwards still has to be received below.
		closeErr := conn.CloseWrite()

		// A detach or a failed stdin copy ends the attach right away.
		if err != nil {
			if !errors.Is(err, detach.ErrDetach) {
				err = fmt.Errorf("copy stdin: %w", err)
			}

			return errors.Join(closeErr, err)
		}

		if closeErr != nil {
			c.logger.Errorf("Unable to close conn: %v", closeErr)
		}

		// The server never closes a terminal, and closes the container stdin
		// without one only with StopAfterStdinEOF. The EOF of the client then
		// only ends this attach session, so stop receiving the output as
		// well, so that nothing gets written to the output streams after the
		// attach returned.
		if (cfg.Tty && cfg.StopAfterStdinEOF) || (!cfg.Tty && cfg.ContainerStdin && !cfg.StopAfterStdinEOF) {
			closeRead()

			if err := c.waitForOutput(ctx, closeRead, receiveStdoutError); err != nil {
				c.logger.WithError(err).Debug("Output failed after stdin EOF")
			}

			return nil
		}

		if cfg.Streams.Stdout != nil || cfg.Streams.Stderr != nil {
			return c.waitForOutput(ctx, closeRead, receiveStdoutError)
		}

	case <-ctx.Done():
		c.logger.WithError(ctx.Err()).Trace("Context done while attached")

		// Closing stdin ends its copy.
		c.tryCloseAttachReaderForID(id)
		c.stopOutput(closeRead, receiveStdoutError)

		return fmt.Errorf("attach: %w", ctx.Err())
	}

	return nil
}

// waitForOutput waits until the output got received. If the context is done
// before, the output gets stopped, closeRead closes the read side of the
// connection.
func (c *ConmonClient) waitForOutput(ctx context.Context, closeRead func(), receiveStdoutError <-chan error) error {
	select {
	case err := <-receiveStdoutError:
		return err
	case <-ctx.Done():
		c.stopOutput(closeRead, receiveStdoutError)

		return fmt.Errorf("wait for output: %w", ctx.Err())
	}
}

// stopOutput stops receiving the output and waits for the output goroutine,
// which closes the output streams, so that nothing writes to or closes them
// after the attach returned. A write blocked by a stream nobody reads can
// only be waited for a while, it may still finish afterwards.
func (c *ConmonClient) stopOutput(closeRead func(), receiveStdoutError <-chan error) {
	closeRead()

	select {
	case err := <-receiveStdoutError:
		c.logger.WithError(err).Trace("Output stopped")
	case <-time.After(attachOutputStopTimeout):
		c.logger.Warn("Output still blocked, returning anyway")
	}
}

// SetWindowSizeContainerConfig is the configuration for calling the SetWindowSizeContainer method.
type SetWindowSizeContainerConfig struct {
	// ID specifies the container ID.
	ID string

	// Size is the new terminal size.
	Size *resize.TerminalSize
}

// SetWindowSizeContainer can be used to change the window size of a running container.
func (c *ConmonClient) SetWindowSizeContainer(ctx context.Context, cfg *SetWindowSizeContainerConfig) error {
	ctx, span := c.startSpan(ctx, "SetWindowSizeContainer")
	if span != nil {
		defer span.End()
	}

	if cfg.Size == nil {
		return errTerminalSizeNil
	}

	rpcClient, err := c.client(ctx)
	if err != nil {
		return fmt.Errorf("create RPC client: %w", err)
	}

	future, free := rpcClient.SetWindowSizeContainer(ctx, func(p proto.Conmon_setWindowSizeContainer_Params) error {
		req, err := p.NewRequest()
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		if err := c.setMetadata(ctx, req); err != nil {
			return err
		}

		if err := req.SetId(cfg.ID); err != nil {
			return fmt.Errorf("set ID: %w", err)
		}

		req.SetWidth(cfg.Size.Width)
		req.SetHeight(cfg.Size.Height)

		return nil
	})
	defer free()

	result, err := future.Struct()
	if err != nil {
		return fmt.Errorf("get result: %w", err)
	}

	if _, err := result.Response(); err != nil {
		return fmt.Errorf("get response: %w", err)
	}

	return nil
}
