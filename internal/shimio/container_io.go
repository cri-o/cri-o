package shimio

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// StreamType is the type of the stream, stdin, stdout or stderr.
type StreamType string

const (
	// Stdin stream type.
	Stdin StreamType = "stdin"
	// Stdout stream type.
	Stdout = StreamType(types.Stdout)
	// Stderr stream type.
	Stderr = StreamType(types.Stderr)
)

// AttachOptions specifies how to attach to a container.
type AttachOptions struct {
	Stdin     io.Reader
	Stdout    io.WriteCloser
	Stderr    io.WriteCloser
	Tty       bool
	StdinOnce bool
	// CloseStdin is the function to close container stdin.
	CloseStdin func() error
}

// streamKey generates a key for the stream.
func streamKey(id, name string, stream StreamType) string {
	return strings.Join([]string{id, name, string(stream)}, "-")
}

// ContainerIO holds the container IO.
type ContainerIO struct {
	*stdioPipes

	id string

	fifos *FIFOSet

	stdoutGroup *writerGroup
	stderrGroup *writerGroup

	closer *wgCloser
}

// ContainerIOOpts sets specific information to newly created ContainerIO.
type ContainerIOOpts func(*ContainerIO) error

// WithFIFOs specifies existing FIFOs for the container IO.
func WithFIFOs(fifos *FIFOSet) ContainerIOOpts {
	return func(c *ContainerIO) error {
		c.fifos = fifos

		return nil
	}
}

// WithNewFIFOs creates new FIFOs for the container IO.
func WithNewFIFOs(root string, tty, stdin bool) ContainerIOOpts {
	return func(c *ContainerIO) error {
		fifos, err := newFifos(root, c.id, tty, stdin)
		if err != nil {
			return err
		}

		return WithFIFOs(fifos)(c)
	}
}

// NewContainerIO creates the container IO.
func NewContainerIO(id string, opts ...ContainerIOOpts) (*ContainerIO, error) {
	c := &ContainerIO{
		id:          id,
		stdoutGroup: newWriterGroup(),
		stderrGroup: newWriterGroup(),
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	if c.fifos == nil {
		return nil, errors.New("fifos are not set")
	}

	stdio, closer, err := newStdioPipes(c.fifos)
	if err != nil {
		return nil, err
	}

	c.stdioPipes = stdio
	c.closer = closer

	return c, nil
}

// Config returns the IO config.
func (c *ContainerIO) Config() Config {
	return c.fifos.Config
}

// Pipe pipes the container output to the output streams.
func (c *ContainerIO) Pipe() {
	wg := c.closer.wg

	if c.stdout != nil {
		wg.Go(func() {
			if _, err := io.Copy(c.stdoutGroup, c.stdout); err != nil {
				logrus.WithError(err).Errorf("Failed to pipe stdout of container %q", c.id)
			}

			c.stdout.Close()
			c.stdoutGroup.Close()
			logrus.Debugf("Finish piping stdout of container %q", c.id)
		})
	}

	if !c.fifos.Terminal && c.stderr != nil {
		wg.Go(func() {
			if _, err := io.Copy(c.stderrGroup, c.stderr); err != nil {
				logrus.WithError(err).Errorf("Failed to pipe stderr of container %q", c.id)
			}

			c.stderr.Close()
			c.stderrGroup.Close()
			logrus.Debugf("Finish piping stderr of container %q", c.id)
		})
	}
}

// Attach attaches the container stdio.
func (c *ContainerIO) Attach(ctx context.Context, opts AttachOptions) {
	var (
		wg            sync.WaitGroup
		stdinStreamRC io.ReadCloser
	)

	key := rand.Text()
	stdinKey := streamKey(c.id, "attach-"+key, Stdin)
	stdoutKey := streamKey(c.id, "attach-"+key, Stdout)
	stderrKey := streamKey(c.id, "attach-"+key, Stderr)

	if c.stdin != nil && opts.Stdin != nil {
		// Create a wrapper of stdin which could be closed. Note that the
		// wrapper doesn't close the actual stdin, it only stops io.Copy.
		// The actual stdin will be closed by stream server.
		stdinStreamRC = newWrapReadCloser(opts.Stdin)

		wg.Go(func() {
			if _, err := io.Copy(c.stdin, stdinStreamRC); err != nil {
				logrus.WithError(err).Errorf("Failed to pipe stdin for container attach %q", c.id)
			}

			logrus.Infof("Attach stream %q closed", stdinKey)

			if opts.StdinOnce && !opts.Tty {
				// Due to kubectl requirements and current docker behavior,
				// when (opts.StdinOnce && opts.Tty) we have to close
				// container stdin and keep stdout and stderr open until
				// container stops.
				c.stdin.Close()
				// Also closes the runtime side.
				if err := opts.CloseStdin(); err != nil {
					logrus.WithError(err).Errorf("Failed to close stdin for container %q", c.id)
				}

				return
			}

			if opts.Stdout != nil {
				c.stdoutGroup.Remove(stdoutKey)
			}

			if opts.Stderr != nil {
				c.stderrGroup.Remove(stderrKey)
			}
		})
	}

	attachStream := func(key string, closeCh <-chan struct{}) {
		select {
		case <-closeCh:
			logrus.Infof("Attach stream %q closed", key)
		case <-ctx.Done():
			logrus.Infof("Attach client of %q cancelled", key)
			// Avoid writeGroup heap up
			c.stdoutGroup.Remove(key)
			c.stderrGroup.Remove(key)
		}
		// Make sure stdin gets closed.
		if stdinStreamRC != nil {
			stdinStreamRC.Close()
		}
	}

	if opts.Stdout != nil {
		wc, closeCh := newWriteCloseInformer(opts.Stdout)
		c.stdoutGroup.Add(stdoutKey, wc)

		wg.Go(func() { attachStream(stdoutKey, closeCh) })
	}

	if !opts.Tty && opts.Stderr != nil {
		wc, closeCh := newWriteCloseInformer(opts.Stderr)
		c.stderrGroup.Add(stderrKey, wc)

		wg.Go(func() { attachStream(stderrKey, closeCh) })
	}

	wg.Wait()
}

// AddOutput adds new write closers to the container stream, and returns
// existing write closers if there are any.
func (c *ContainerIO) AddOutput(
	name string,
	stdout, stderr io.WriteCloser,
) (oldStdout, oldStderr io.WriteCloser) {
	if stdout != nil {
		key := streamKey(c.id, name, Stdout)
		oldStdout = c.stdoutGroup.Get(key)
		c.stdoutGroup.Add(key, stdout)
	}

	if stderr != nil {
		key := streamKey(c.id, name, Stderr)
		oldStderr = c.stderrGroup.Get(key)
		c.stderrGroup.Add(key, stderr)
	}

	return oldStdout, oldStderr
}

// Cancel cancels the container IO.
func (c *ContainerIO) Cancel() {
	c.closer.Cancel()
}

// Wait waits for the container IO to finish.
func (c *ContainerIO) Wait() {
	c.closer.Wait()
}

// Close closes all FIFOs.
func (c *ContainerIO) Close() error {
	c.closer.Close()

	if c.fifos != nil {
		return c.fifos.Close()
	}

	return nil
}
