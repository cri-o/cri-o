package events

import (
	"fmt"
	"os"

	"github.com/containers/storage"
	"github.com/pkg/errors"
)

// EventLogFile is the structure for event writing to a logfile. It contains the eventer
// options and the event itself.  Methods for reading and writing are also defined from it.
type EventLogFile struct {
	options EventerOptions
}

// Writes to the log file
func (e EventLogFile) Write(ee Event) error {
	// We need to lock events file
	lock, err := storage.GetLockfile(e.options.LogFilePath + ".lock")
	if err != nil {
		return err
	}
	lock.Lock()
	defer lock.Unlock()
	f, err := os.OpenFile(e.options.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	defer f.Close()
	eventJSONString, err := ee.ToJSONString()
	if err != nil {
		return err
	}
	if _, err := f.WriteString(fmt.Sprintf("%s\n", eventJSONString)); err != nil {
		return err
	}
	return nil

}

// Reads from the log file
func (e EventLogFile) Read(options ReadOptions) error {
	defer close(options.EventChannel)
	eventOptions, err := generateEventOptions(options.Filters, options.Since, options.Until)
	if err != nil {
		return errors.Wrapf(err, "unable to generate event options")
	}
	t, err := e.getTail(options)
	if err != nil {
		return err
	}
	for line := range t.Lines {
		event, err := newEventFromJSONString(line.Text)
		if err != nil {
			return err
		}
		switch event.Type {
		case Image, Volume, Pod, System, Container:
		//	no-op
		default:
			return errors.Errorf("event type %s is not valid in %s", event.Type.String(), e.options.LogFilePath)
		}
		include := true
		for _, filter := range eventOptions {
			include = include && filter(event)
		}
		if include {
			options.EventChannel <- event
		}
	}
	return nil
}

// String returns a string representation of the logger
func (e EventLogFile) String() string {
	return LogFile.String()
}
