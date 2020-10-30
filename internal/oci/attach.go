package oci

import (
	"io"

	conmon "github.com/containers/conmon/runner/config"
	"github.com/sirupsen/logrus"
)

/* Sync with stdpipe_t in conmon.c */
const (
	AttachPipeStdin  = 1
	AttachPipeStdout = 2
	AttachPipeStderr = 3
)

func redirectResponseToOutputStreams(outputStream, errorStream io.WriteCloser, conn io.Reader) error {
	var err error
	buf := make([]byte, conmon.BufSize+1)

	for {
		nr, er := conn.Read(buf)
		if nr > 0 {
			var dst io.Writer
			switch buf[0] {
			case AttachPipeStdout:
				dst = outputStream
			case AttachPipeStderr:
				dst = errorStream
			default:
				logrus.Debugf("Got unexpected attach type %+d", buf[0])
			}
			if dst != nil {
				nw, ew := dst.Write(buf[1:nr])
				if ew != nil {
					err = ew
					break
				}
				if nr != nw+1 {
					err = io.ErrShortWrite
					break
				}
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			err = er
			break
		}
	}

	return err
}
