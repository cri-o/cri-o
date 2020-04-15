package libpod

import (
	"fmt"

	"github.com/containers/libpod/libpod/events"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// newEventer returns an eventer that can be used to read/write events
func (r *Runtime) newEventer() (events.Eventer, error) {
	options := events.EventerOptions{
		EventerType: r.config.Engine.EventsLogger,
		LogFilePath: r.config.Engine.EventsLogFilePath,
	}
	return events.NewEventer(options)
}

// newContainerEvent creates a new event based on a container
func (c *Container) newContainerEvent(status events.Status) {
	e := events.NewEvent(status)
	e.ID = c.ID()
	e.Name = c.Name()
	e.Image = c.config.RootfsImageName
	e.Type = events.Container
	if err := c.runtime.eventer.Write(e); err != nil {
		logrus.Errorf("unable to write pod event: %q", err)
	}
}

// newContainerExitedEvent creates a new event for a container's death
func (c *Container) newContainerExitedEvent(exitCode int32) {
	e := events.NewEvent(events.Exited)
	e.ID = c.ID()
	e.Name = c.Name()
	e.Image = c.config.RootfsImageName
	e.Type = events.Container
	e.ContainerExitCode = int(exitCode)
	if err := c.runtime.eventer.Write(e); err != nil {
		logrus.Errorf("unable to write pod event: %q", err)
	}
}

// newPodEvent creates a new event for a libpod pod
func (p *Pod) newPodEvent(status events.Status) {
	e := events.NewEvent(status)
	e.ID = p.ID()
	e.Name = p.Name()
	e.Type = events.Pod
	if err := p.runtime.eventer.Write(e); err != nil {
		logrus.Errorf("unable to write pod event: %q", err)
	}
}

// newSystemEvent creates a new event for libpod as a whole.
func (r *Runtime) newSystemEvent(status events.Status) {
	e := events.NewEvent(status)
	e.Type = events.System

	if err := r.eventer.Write(e); err != nil {
		logrus.Errorf("unable to write system event: %q", err)
	}
}

// newVolumeEvent creates a new event for a libpod volume
func (v *Volume) newVolumeEvent(status events.Status) {
	e := events.NewEvent(status)
	e.Name = v.Name()
	e.Type = events.Volume
	if err := v.runtime.eventer.Write(e); err != nil {
		logrus.Errorf("unable to write volume event: %q", err)
	}
}

// Events is a wrapper function for everyone to begin tailing the events log
// with options
func (r *Runtime) Events(options events.ReadOptions) error {
	eventer, err := r.newEventer()
	if err != nil {
		return err
	}
	return eventer.Read(options)
}

// GetEvents reads the event log and returns events based on input filters
func (r *Runtime) GetEvents(filters []string) ([]*events.Event, error) {
	var (
		logEvents []*events.Event
		readErr   error
	)
	eventChannel := make(chan *events.Event)
	options := events.ReadOptions{
		EventChannel: eventChannel,
		Filters:      filters,
		FromStart:    true,
		Stream:       false,
	}
	eventer, err := r.newEventer()
	if err != nil {
		return nil, err
	}
	go func() {
		readErr = eventer.Read(options)
	}()
	if readErr != nil {
		return nil, readErr
	}
	for e := range eventChannel {
		logEvents = append(logEvents, e)
	}
	return logEvents, nil
}

// GetLastContainerEvent takes a container name or ID and an event status and returns
// the last occurrence of the container event
func (r *Runtime) GetLastContainerEvent(nameOrID string, containerEvent events.Status) (*events.Event, error) {
	// check to make sure the event.Status is valid
	if _, err := events.StringToStatus(containerEvent.String()); err != nil {
		return nil, err
	}
	filters := []string{
		fmt.Sprintf("container=%s", nameOrID),
		fmt.Sprintf("event=%s", containerEvent),
		"type=container",
	}
	containerEvents, err := r.GetEvents(filters)
	if err != nil {
		return nil, err
	}
	if len(containerEvents) < 1 {
		return nil, errors.Wrapf(events.ErrEventNotFound, "%s not found", containerEvent.String())
	}
	// return the last element in the slice
	return containerEvents[len(containerEvents)-1], nil
}
