package ioutil

import "io"

// writeCloseInformer wraps a reader with a close function.
type wrapReadCloser struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

// NewWrapReadCloser creates a wrapReadCloser from a reader.
// NOTE(random-liu): To avoid goroutine leakage, the reader passed in
// must be eventually closed by the caller.
func NewWrapReadCloser(r io.Reader) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		_, _ = io.Copy(pw, r)
		pr.Close()
		pw.Close()
	}()
	return &wrapReadCloser{
		reader: pr,
		writer: pw,
	}
}

// Read reads up to len(p) bytes into p.
func (w *wrapReadCloser) Read(p []byte) (int, error) {
	n, err := w.reader.Read(p)
	if err == io.ErrClosedPipe {
		return n, io.EOF
	}
	return n, err
}

// Close closes read closer.
func (w *wrapReadCloser) Close() error {
	w.reader.Close()
	w.writer.Close()
	return nil
}
