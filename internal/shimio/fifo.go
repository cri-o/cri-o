package shimio

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/containerd/fifo"
)

// Config holds the IO configuration of a task or process.
type Config struct {
	// Terminal is true if one has been allocated.
	Terminal bool
	// Stdin is the stdin FIFO path.
	Stdin string
	// Stdout is the stdout FIFO path.
	Stdout string
	// Stderr is the stderr FIFO path.
	Stderr string
}

// FIFOSet is a set of file paths to FIFOs for a task's standard IO streams.
type FIFOSet struct {
	Config

	close func() error
}

// NewFIFOSet returns a new FIFOSet from a Config and a close function.
func NewFIFOSet(config Config, closeFn func() error) *FIFOSet {
	return &FIFOSet{Config: config, close: closeFn}
}

// NewFIFOSetInDir returns a new FIFOSet with paths in a temporary directory
// under root.
func NewFIFOSetInDir(root, id string, terminal bool) (*FIFOSet, error) {
	if root != "" {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return nil, err
		}
	}

	dir, err := os.MkdirTemp(root, "")
	if err != nil {
		return nil, err
	}

	return NewFIFOSet(Config{
		Stdin:    filepath.Join(dir, id+"-stdin"),
		Stdout:   filepath.Join(dir, id+"-stdout"),
		Stderr:   filepath.Join(dir, id+"-stderr"),
		Terminal: terminal,
	}, func() error {
		return os.RemoveAll(dir)
	}), nil
}

// Close the FIFOSet.
func (f *FIFOSet) Close() error {
	if f != nil && f.close != nil {
		return f.close()
	}

	return nil
}

// newFifos creates the FIFO directory for a container.
func newFifos(root, id string, tty, stdin bool) (*FIFOSet, error) {
	root = filepath.Join(root, "io")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}

	fifos, err := NewFIFOSetInDir(root, id, tty)
	if err != nil {
		return nil, err
	}

	if !stdin {
		fifos.Stdin = ""
	}

	return fifos, nil
}

type stdioPipes struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

type wgCloser struct {
	wg     *sync.WaitGroup
	set    []io.Closer
	cancel context.CancelFunc
}

func (g *wgCloser) Wait() {
	g.wg.Wait()
}

func (g *wgCloser) Close() {
	for _, f := range g.set {
		f.Close()
	}
}

func (g *wgCloser) Cancel() {
	g.cancel()
}

// newStdioPipes opens the FIFOs for stdio.
func newStdioPipes(fifos *FIFOSet) (_ *stdioPipes, _ *wgCloser, retErr error) {
	var (
		set         []io.Closer
		ctx, cancel = context.WithCancel(context.Background())
		p           = &stdioPipes{}
	)

	defer func() {
		if retErr != nil {
			for _, f := range set {
				f.Close()
			}

			cancel()
		}
	}()

	open := func(path string, flag int) (io.ReadWriteCloser, error) {
		f, err := fifo.OpenFifo(ctx, path, flag|syscall.O_CREAT|syscall.O_NONBLOCK, 0o700)
		if err != nil {
			return nil, err
		}

		set = append(set, f)

		return f, nil
	}

	if fifos.Stdin != "" {
		f, err := open(fifos.Stdin, syscall.O_WRONLY)
		if err != nil {
			return nil, nil, err
		}

		p.stdin = f
	}

	if fifos.Stdout != "" {
		f, err := open(fifos.Stdout, syscall.O_RDONLY)
		if err != nil {
			return nil, nil, err
		}

		p.stdout = f
	}

	if fifos.Stderr != "" {
		f, err := open(fifos.Stderr, syscall.O_RDONLY)
		if err != nil {
			return nil, nil, err
		}

		p.stderr = f
	}

	return p, &wgCloser{
		wg:     &sync.WaitGroup{},
		set:    set,
		cancel: cancel,
	}, nil
}
