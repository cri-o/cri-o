package shimio

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	// delimiter used in CRI logging format.
	delimiter = ' '
	// eol is end-of-line.
	eol = '\n'
	// timestampFormat is the timestamp format used in CRI logging format.
	timestampFormat = time.RFC3339Nano
	// defaultBufSize is the default size of the read buffer in bytes.
	defaultBufSize = 4096
)

// NewCRILogger returns a write closer which redirects the container log into
// a log file and decorates each line in the CRI defined format. It also
// returns a channel which indicates whether the logger is stopped. Lines are
// not length limited.
func NewCRILogger(
	path string, w io.Writer, stream StreamType,
) (logger io.WriteCloser, stopped <-chan struct{}) {
	logrus.Debugf("Start writing stream %q to log file %q", stream, path)

	prc, pwc := io.Pipe()
	stop := make(chan struct{})

	go func() {
		redirectLogs(path, prc, w, stream)
		close(stop)
	}()

	return pwc, stop
}

// readLine is like bufio.Reader.ReadLine, but does not eat read errors and
// trailing newlines. bufio.ReadLine makes it impossible to figure out whether
// a line read until io.EOF ended with a newline or not.
//
// readLine returns err != nil if and only if line does not end with a new
// line.
func readLine(b *bufio.Reader) (line []byte, isPrefix bool, err error) {
	line, err = b.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		// Handle the case where "\r\n" straddles the buffer.
		if len(line) > 0 && line[len(line)-1] == '\r' {
			// Unread the last '\r'
			if err := b.UnreadByte(); err != nil {
				panic(fmt.Sprintf("invalid unread %v", err))
			}

			line = line[:len(line)-1]
		}

		return line, true, nil
	}

	if len(line) == 0 {
		if err != nil {
			line = nil
		}

		return line, false, err
	}

	if line[len(line)-1] == '\n' {
		// "ReadSlice returns err != nil if and only if line does not end in delim"
		// (See https://golang.org/pkg/bufio/#Reader.ReadSlice).
		if err != nil {
			panic(fmt.Sprintf("full read with unexpected error %v", err))
		}

		drop := 1
		if len(line) > 1 && line[len(line)-2] == '\r' {
			drop = 2
		}

		line = line[:len(line)-drop]
	}

	return line, false, err
}

func redirectLogs(path string, rc io.ReadCloser, w io.Writer, s StreamType) {
	defer rc.Close()

	var (
		stream     = []byte(s)
		delim      = []byte{delimiter}
		partial    = []byte(types.LogTagPartial)
		full       = []byte(types.LogTagFull)
		buf        [][]byte
		timeBuffer = make([]byte, len(timestampFormat))
		lineBuffer = bytes.Buffer{}
	)

	r := bufio.NewReaderSize(rc, defaultBufSize)

	writeLineBuffer := func(tag []byte, lineBytes [][]byte) {
		timeBuffer = time.Now().AppendFormat(timeBuffer[:0], timestampFormat)
		headers := [][]byte{timeBuffer, stream, tag}

		lineBuffer.Reset()

		for _, h := range headers {
			lineBuffer.Write(h)
			lineBuffer.Write(delim)
		}

		for _, l := range lineBytes {
			lineBuffer.Write(l)
		}

		lineBuffer.WriteByte(eol)

		if _, err := lineBuffer.WriteTo(w); err != nil {
			logrus.WithError(err).Errorf("Fail to write %q log to log file %q", s, path)
			// Continue on write error to drain the container output.
		}
	}

	for {
		var stop bool

		newLine, isPrefix, err := readLine(r)
		// readLine can return actual content even if there is an error.
		if len(newLine) > 0 {
			// Buffer returned by ReadLine will change after next read, copy it.
			l := make([]byte, len(newLine))
			copy(l, newLine)
			buf = append(buf, l)
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				logrus.Tracef(
					"Getting EOF from stream %q while redirecting to log file %q",
					s,
					path,
				)
			} else {
				logrus.WithError(err).
					Errorf("An error occurred when redirecting stream %q to log file %q", s, path)
			}

			if len(buf) == 0 {
				// No content left to write, break.
				break
			}
			// Stop after writing the content left in buffer.
			stop = true
		}

		if isPrefix {
			continue
		}

		if stop {
			// readLine only returns error when the message doesn't end with
			// a newline, in that case it should be treated as a partial line.
			writeLineBuffer(partial, buf)
		} else {
			writeLineBuffer(full, buf)
		}

		buf = nil

		if stop {
			break
		}
	}

	logrus.Debugf("Finish redirecting stream %q to log file %q", s, path)
}
