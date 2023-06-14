package utils

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/cri-o/cri-o/internal/dbusmgr"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"

	systemdDbus "github.com/coreos/go-systemd/v22/dbus"
)

// RunUnderSystemdScope adds the specified pid to a systemd scope
func RunUnderSystemdScope(mgr *dbusmgr.DbusConnManager, pid int, slice, unitName string, properties ...systemdDbus.Property) (err error) {
	ctx := context.Background()
	// sanity check
	if mgr == nil {
		return errors.New("dbus manager is nil")
	}
	defaultProperties := []systemdDbus.Property{
		newProp("PIDs", []uint32{uint32(pid)}),
		newProp("Delegate", true),
		newProp("DefaultDependencies", false),
	}
	properties = append(defaultProperties, properties...)
	if slice != "" {
		properties = append(properties, systemdDbus.PropSlice(slice))
	}
	// Make a buffered channel so that the sender (go-systemd's jobComplete)
	// won't be blocked on channel send while holding the jobListener lock
	// (RHBZ#2082344).
	ch := make(chan string, 1)
	if err := mgr.RetryOnDisconnect(func(c *systemdDbus.Conn) error {
		_, err = c.StartTransientUnitContext(ctx, unitName, "replace", properties, ch)
		return err
	}); err != nil {
		return fmt.Errorf("start transient unit %q: %w", unitName, err)
	}

	// Wait for the job status.
	select {
	case s := <-ch:
		close(ch)
		if s != "done" {
			return fmt.Errorf("error moving conmon with pid %d to systemd unit %s: got %s", pid, unitName, s)
		}
	case <-time.After(time.Minute * 6):
		// This case is a work around to catch situations where the dbus library sends the
		// request but it unexpectedly disappears. We set the timeout very high to make sure
		// we wait as long as possible to catch situations where dbus is overwhelmed.
		// We also don't use the native context cancelling behavior of the dbus library,
		// because experience has shown that it does not help.
		// TODO: Find cause of the request being dropped in the dbus library and fix it.
		return fmt.Errorf("timed out moving conmon with pid %d to systemd unit %s", pid, unitName)
	}

	return nil
}

// Syncfs ensures the file system at path is synced to disk
func Syncfs(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY, 0o755)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := unix.Syncfs(int(f.Fd())); err != nil {
		return err
	}
	return nil
}
