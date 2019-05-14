package events

import (
	"fmt"
	"time"

	"github.com/coreos/go-systemd/journal"
	"github.com/coreos/go-systemd/sdjournal"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// EventJournalD is the journald implementation of an eventer
type EventJournalD struct {
	options EventerOptions
}

// Write to journald
func (e EventJournalD) Write(ee Event) error {
	m := make(map[string]string)
	m["SYSLOG_IDENTIFIER"] = "podman"
	m["PODMAN_EVENT"] = ee.Status.String()
	m["PODMAN_TYPE"] = ee.Type.String()
	m["PODMAN_TIME"] = ee.Time.Format(time.RFC3339Nano)

	// Add specialized information based on the podman type
	switch ee.Type {
	case Image:
		m["PODMAN_NAME"] = ee.Name
		m["PODMAN_ID"] = ee.ID
	case Container, Pod:
		m["PODMAN_IMAGE"] = ee.Image
		m["PODMAN_NAME"] = ee.Name
		m["PODMAN_ID"] = ee.ID
	case Volume:
		m["PODMAN_NAME"] = ee.Name
	}
	return journal.Send(fmt.Sprintf("%s", ee.ToHumanReadable()), journal.PriInfo, m)
}

// Read reads events from the journal and sends qualified events to the event channel
func (e EventJournalD) Read(options ReadOptions) error {
	eventOptions, err := generateEventOptions(options.Filters, options.Since, options.Until)
	if err != nil {
		return errors.Wrapf(err, "failed to generate event options")
	}
	podmanJournal := sdjournal.Match{Field: "SYSLOG_IDENTIFIER", Value: "podman"} //nolint
	j, err := sdjournal.NewJournal()                                              //nolint
	if err != nil {
		return err
	}
	if err := j.AddMatch(podmanJournal.String()); err != nil {
		return errors.Wrap(err, "failed to add filter for event log")
	}
	if len(options.Since) == 0 && len(options.Until) == 0 && options.Stream {
		if err := j.SeekTail(); err != nil {
			return errors.Wrap(err, "failed to seek end of journal")
		}
	}
	// the api requires a next|prev before getting a cursor
	if _, err := j.Next(); err != nil {
		return err
	}
	prevCursor, err := j.GetCursor()
	if err != nil {
		return err
	}
	defer close(options.EventChannel)
	for {
		if _, err := j.Next(); err != nil {
			return err
		}
		newCursor, err := j.GetCursor()
		if err != nil {
			return err
		}
		if prevCursor == newCursor {
			if len(options.Until) > 0 || !options.Stream {
				break
			}
			_ = j.Wait(sdjournal.IndefiniteWait) //nolint
			continue
		}
		prevCursor = newCursor
		entry, err := j.GetEntry()
		if err != nil {
			return err
		}
		newEvent, err := newEventFromJournalEntry(entry)
		if err != nil {
			// We can't decode this event.
			// Don't fail hard - that would make events unusable.
			// Instead, log and continue.
			logrus.Errorf("Unable to decode event: %v", err)
			continue
		}
		include := true
		for _, filter := range eventOptions {
			include = include && filter(newEvent)
		}
		if include {
			options.EventChannel <- newEvent
		}
	}
	return nil

}

func newEventFromJournalEntry(entry *sdjournal.JournalEntry) (*Event, error) { //nolint
	newEvent := Event{}
	eventType, err := StringToType(entry.Fields["PODMAN_TYPE"])
	if err != nil {
		return nil, err
	}
	eventTime, err := time.Parse(time.RFC3339Nano, entry.Fields["PODMAN_TIME"])
	if err != nil {
		return nil, err
	}
	eventStatus, err := StringToStatus(entry.Fields["PODMAN_EVENT"])
	if err != nil {
		return nil, err
	}
	newEvent.Type = eventType
	newEvent.Time = eventTime
	newEvent.Status = eventStatus
	newEvent.Name = entry.Fields["PODMAN_NAME"]

	switch eventType {
	case Container, Pod:
		newEvent.ID = entry.Fields["PODMAN_ID"]
		newEvent.Image = entry.Fields["PODMAN_IMAGE"]
	case Image:
		newEvent.ID = entry.Fields["PODMAN_ID"]
	}
	return &newEvent, nil
}
