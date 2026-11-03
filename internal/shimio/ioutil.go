package shimio

import (
	"errors"
	"io"
	"sync"
)

// writerGroup is a group of writers. Writers can be dynamically added and
// removed.
type writerGroup struct {
	mu      sync.Mutex
	writers map[string]io.WriteCloser
	closed  bool
}

func newWriterGroup() *writerGroup {
	return &writerGroup{
		writers: make(map[string]io.WriteCloser),
	}
}

// Add adds a writer into the group. The writer will be closed if the writer
// group is closed.
func (g *writerGroup) Add(key string, w io.WriteCloser) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.closed {
		w.Close()

		return
	}

	g.writers[key] = w
}

// Get gets a writer from the group, returns nil if the writer doesn't exist.
func (g *writerGroup) Get(key string) io.WriteCloser {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.writers[key]
}

// Remove removes a writer from the group.
func (g *writerGroup) Remove(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	w, ok := g.writers[key]
	if !ok {
		return
	}

	w.Close()
	delete(g.writers, key)
}

// Write writes data into each writer. If a writer returns an error, it will be
// closed and removed from the writer group. It returns an error if the writer
// group is empty.
func (g *writerGroup) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for k, w := range g.writers {
		n, err := w.Write(p)
		if err == nil && len(p) == n {
			continue
		}
		// The writer is closed or in bad state, remove it.
		w.Close()
		delete(g.writers, k)
	}

	if len(g.writers) == 0 {
		return 0, errors.New("writer group is empty")
	}

	return len(p), nil
}

// Close closes the writer group. Write will return an error afterwards.
func (g *writerGroup) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, w := range g.writers {
		w.Close()
	}

	g.writers = nil
	g.closed = true
}

// wrapReadCloser wraps a reader with a close function.
type wrapReadCloser struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

// newWrapReadCloser creates a wrapReadCloser from a reader. To avoid goroutine
// leakage, the reader passed in must be eventually closed by the caller.
func newWrapReadCloser(r io.Reader) io.ReadCloser {
	pr, pw := io.Pipe()

	go func() {
		//nolint:errcheck // the error surfaces on the reading side
		io.Copy(pw, r)
		pr.Close()
		pw.Close()
	}()

	return &wrapReadCloser{
		reader: pr,
		writer: pw,
	}
}

func (w *wrapReadCloser) Read(p []byte) (int, error) {
	n, err := w.reader.Read(p)
	if errors.Is(err, io.ErrClosedPipe) {
		return n, io.EOF
	}

	return n, err
}

func (w *wrapReadCloser) Close() error {
	w.reader.Close()
	w.writer.Close()

	return nil
}

// writeCloseInformer wraps a write closer with a close channel. Callers can
// wait on the channel for the write closer to be closed.
type writeCloseInformer struct {
	close chan struct{}
	wc    io.WriteCloser
}

func newWriteCloseInformer(wc io.WriteCloser) (informer io.WriteCloser, closeCh <-chan struct{}) {
	ch := make(chan struct{})

	return &writeCloseInformer{
		close: ch,
		wc:    wc,
	}, ch
}

func (w *writeCloseInformer) Write(p []byte) (int, error) {
	return w.wc.Write(p)
}

func (w *writeCloseInformer) Close() error {
	err := w.wc.Close()
	close(w.close)

	return err
}

// serialWriteCloser wraps a write closer and makes sure all writes are done
// serially, so that parallel writes won't intersect with each other.
type serialWriteCloser struct {
	mu sync.Mutex
	wc io.WriteCloser
}

// NewSerialWriteCloser creates a write closer which serializes all writes to
// wc.
func NewSerialWriteCloser(wc io.WriteCloser) io.WriteCloser {
	return &serialWriteCloser{wc: wc}
}

func (s *serialWriteCloser) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.wc.Write(data)
}

func (s *serialWriteCloser) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.wc.Close()
}
