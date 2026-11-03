package shimio

import (
	"io"
	"sync"

	"github.com/sirupsen/logrus"
)

// ExecIO holds the exec IO.
type ExecIO struct {
	*stdioPipes

	id     string
	fifos  *FIFOSet
	closer *wgCloser
}

// NewExecIO creates the exec IO.
func NewExecIO(id, root string, tty, stdin bool) (*ExecIO, error) {
	fifos, err := newFifos(root, id, tty, stdin)
	if err != nil {
		return nil, err
	}

	stdio, closer, err := newStdioPipes(fifos)
	if err != nil {
		return nil, err
	}

	return &ExecIO{
		id:         id,
		fifos:      fifos,
		stdioPipes: stdio,
		closer:     closer,
	}, nil
}

// Config returns the IO config.
func (e *ExecIO) Config() Config {
	return e.fifos.Config
}

// Attach attaches the exec stdio. The logic is similar to the container IO
// attach.
func (e *ExecIO) Attach(opts AttachOptions) <-chan struct{} {
	var (
		wg            sync.WaitGroup
		stdinStreamRC io.ReadCloser
	)

	if e.stdin != nil && opts.Stdin != nil {
		stdinStreamRC = newWrapReadCloser(opts.Stdin)

		wg.Go(func() {
			if _, err := io.Copy(e.stdin, stdinStreamRC); err != nil {
				logrus.WithError(err).Errorf("Failed to redirect stdin for container exec %q", e.id)
			}

			logrus.Infof("Container exec %q stdin closed", e.id)

			if opts.StdinOnce && !opts.Tty {
				e.stdin.Close()

				if err := opts.CloseStdin(); err != nil {
					logrus.WithError(err).
						Errorf("Failed to close stdin for container exec %q", e.id)
				}

				return
			}

			if e.stdout != nil {
				e.stdout.Close()
			}

			if e.stderr != nil {
				e.stderr.Close()
			}
		})
	}

	attachOutput := func(t StreamType, stream io.WriteCloser, out io.ReadCloser) {
		if _, err := io.Copy(stream, out); err != nil {
			logrus.WithError(err).Errorf("Failed to pipe %q for container exec %q", t, e.id)
		}

		out.Close()
		stream.Close()

		if stdinStreamRC != nil {
			stdinStreamRC.Close()
		}

		e.closer.wg.Done()
		logrus.Debugf("Finish piping %q of container exec %q", t, e.id)
	}

	if opts.Stdout != nil {
		// Closer should wait for this routine to be over.
		e.closer.wg.Add(1)
		wg.Go(func() { attachOutput(Stdout, opts.Stdout, e.stdout) })
	}

	if !opts.Tty && opts.Stderr != nil {
		// Closer should wait for this routine to be over.
		e.closer.wg.Add(1)
		wg.Go(func() { attachOutput(Stderr, opts.Stderr, e.stderr) })
	}

	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	return done
}

// Cancel cancels the exec IO.
func (e *ExecIO) Cancel() {
	e.closer.Cancel()
}

// Wait waits for the exec IO to finish.
func (e *ExecIO) Wait() {
	e.closer.Wait()
}

// Close closes all FIFOs.
func (e *ExecIO) Close() error {
	if e.closer != nil {
		e.closer.Close()
	}

	if e.fifos != nil {
		return e.fifos.Close()
	}

	return nil
}
