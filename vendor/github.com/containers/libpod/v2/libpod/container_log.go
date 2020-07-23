package libpod

import (
	"fmt"
	"os"
	"time"

	"github.com/containers/libpod/v2/libpod/define"
	"github.com/containers/libpod/v2/libpod/logs"
	"github.com/hpcloud/tail/watch"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Log is a runtime function that can read one or more container logs.
func (r *Runtime) Log(containers []*Container, options *logs.LogOptions, logChannel chan *logs.LogLine) error {
	for _, ctr := range containers {
		if err := ctr.ReadLog(options, logChannel); err != nil {
			return err
		}
	}
	return nil
}

// ReadLog reads a containers log based on the input options and returns loglines over a channel.
func (c *Container) ReadLog(options *logs.LogOptions, logChannel chan *logs.LogLine) error {
	switch c.LogDriver() {
	case define.NoLogging:
		return errors.Wrapf(define.ErrNoLogs, "this container is using the 'none' log driver, cannot read logs")
	case define.JournaldLogging:
		// TODO Skip sending logs until journald logs can be read
		return c.readFromJournal(options, logChannel)
	case define.JSONLogging:
		// TODO provide a separate implementation of this when Conmon
		// has support.
		fallthrough
	case define.KubernetesLogging, "":
		return c.readFromLogFile(options, logChannel)
	default:
		return errors.Wrapf(define.ErrInternal, "unrecognized log driver %q, cannot read logs", c.LogDriver())
	}
}

func (c *Container) readFromLogFile(options *logs.LogOptions, logChannel chan *logs.LogLine) error {
	t, tailLog, err := logs.GetLogFile(c.LogPath(), options)
	if err != nil {
		// If the log file does not exist, this is not fatal.
		if os.IsNotExist(errors.Cause(err)) {
			return nil
		}
		return errors.Wrapf(err, "unable to read log file %s for %s ", c.ID(), c.LogPath())
	}
	options.WaitGroup.Add(1)
	if len(tailLog) > 0 {
		for _, nll := range tailLog {
			nll.CID = c.ID()
			nll.CName = c.Name()
			if nll.Since(options.Since) {
				logChannel <- nll
			}
		}
	}

	go func() {
		var partial string
		for line := range t.Lines {
			nll, err := logs.NewLogLine(line.Text)
			if err != nil {
				logrus.Error(err)
				continue
			}
			if nll.Partial() {
				partial += nll.Msg
				continue
			} else if !nll.Partial() && len(partial) > 1 {
				nll.Msg = partial + nll.Msg
				partial = ""
			}
			nll.CID = c.ID()
			nll.CName = c.Name()
			if nll.Since(options.Since) {
				logChannel <- nll
			}
		}
		options.WaitGroup.Done()
	}()
	// Check if container is still running or paused
	if options.Follow {
		go func() {
			for {
				state, err := c.State()
				time.Sleep(watch.POLL_DURATION)
				if err != nil {
					tailError := t.StopAtEOF()
					if tailError != nil && fmt.Sprintf("%v", tailError) != "tail: stop at eof" {
						logrus.Error(tailError)
					}
					if errors.Cause(err) != define.ErrNoSuchCtr {
						logrus.Error(err)
					}
					break
				}
				if state != define.ContainerStateRunning && state != define.ContainerStatePaused {
					tailError := t.StopAtEOF()
					if tailError != nil && fmt.Sprintf("%v", tailError) != "tail: stop at eof" {
						logrus.Error(tailError)
					}
					break
				}
			}
		}()
	}
	return nil
}
