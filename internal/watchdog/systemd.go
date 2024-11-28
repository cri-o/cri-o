package watchdog

import (
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
)

// Systemd is the main interface for supported systemd functionality.
type Systemd interface {
	WatchdogEnabled() (time.Duration, error)
	Notify(string) (bool, error)
}

type defaultSystemd struct{}

// DefaultSystemd returns the default systemd implementation.
func DefaultSystemd() Systemd {
	return &defaultSystemd{}
}

// WatchdogEnabled returns watchdog information for a service.
// Processes should call Notify(daemon.SdNotifyWatchdog) every
// time / 2.
//
// It returns one of the following:
// (0, nil) - watchdog isn't enabled or we aren't the watched PID.
// (0, err) - an error happened (e.g. error converting time).
// (time, nil) - watchdog is enabled and we can send ping.  time is delay
// before inactive service will be killed.
func (*defaultSystemd) WatchdogEnabled() (time.Duration, error) {
	return daemon.SdWatchdogEnabled(false)
}

// Notify sends a message to the init daemon. It is common to ignore the error.
//
// It returns one of the following:
// (false, nil) - notification not supported (i.e. NOTIFY_SOCKET is unset).
// (false, err) - notification supported, but failure happened (e.g. error connecting to NOTIFY_SOCKET or while sending data).
// (true, nil) - notification supported, data has been sent.
func (d *defaultSystemd) Notify(state string) (bool, error) {
	return daemon.SdNotify(false, state)
}
