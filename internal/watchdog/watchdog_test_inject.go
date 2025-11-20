//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package watchdog

// SetSystemd sets the systemd implementation.
func (w *Watchdog) SetSystemd(systemd Systemd) {
	w.systemd = systemd
}
