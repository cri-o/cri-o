//go:build linux
// +build linux

// Code in this package is heavily adapted from https://github.com/opencontainers/runc/blob/7362fa2d282feffb9b19911150e01e390a23899d/libcontainer/cgroups/systemd
// Credit goes to the runc authors.

package dbusmgr

import (
	"context"
	"errors"
	"sync"
	"syscall"

	systemdDbus "github.com/coreos/go-systemd/v22/dbus"
	dbus "github.com/godbus/dbus/v5"
)

var (
	dbusC        *systemdDbus.Conn
	dbusMu       sync.RWMutex
	dbusInited   bool
	dbusRootless bool
)

type DbusConnManager struct{}

// NewDbusConnManager initializes systemd dbus connection manager.
func NewDbusConnManager(rootless bool) *DbusConnManager {
	dbusMu.Lock()
	defer dbusMu.Unlock()
	if dbusInited && rootless != dbusRootless {
		panic("can't have both root and rootless dbus")
	}
	dbusRootless = rootless
	dbusInited = true
	return &DbusConnManager{}
}

// getConnection lazily initializes and returns systemd dbus connection.
func (d *DbusConnManager) GetConnection() (*systemdDbus.Conn, error) {
	// In the case where dbusC != nil
	// Use the read lock the first time to ensure
	// that Conn can be acquired at the same time.
	dbusMu.RLock()
	if conn := dbusC; conn != nil {
		dbusMu.RUnlock()
		return conn, nil
	}
	dbusMu.RUnlock()

	// In the case where dbusC == nil
	// Use write lock to ensure that only one
	// will be created
	dbusMu.Lock()
	defer dbusMu.Unlock()
	if conn := dbusC; conn != nil {
		return conn, nil
	}

	conn, err := d.newConnection()
	if err != nil {
		return nil, err
	}
	dbusC = conn
	return conn, nil
}

func (d *DbusConnManager) newConnection() (*systemdDbus.Conn, error) {
	if dbusRootless {
		return newUserSystemdDbus()
	}
	return systemdDbus.NewWithContext(context.TODO())
}

// RetryOnDisconnect calls op, and if the error it returns is about closed dbus
// connection, the connection is re-established and the op is retried. This helps
// with the situation when dbus is restarted and we have a stale connection.
func (d *DbusConnManager) RetryOnDisconnect(op func(*systemdDbus.Conn) error) error {
	for {
		conn, err := d.GetConnection()
		if err != nil {
			return err
		}
		err = op(conn)
		if err == nil {
			return nil
		}
		if errors.Is(err, syscall.EAGAIN) {
			continue
		}
		if !errors.Is(err, dbus.ErrClosed) {
			return err
		}
		// dbus connection closed, we should reconnect and try again
		d.resetConnection(conn)
	}
}

// resetConnection resets the connection to its initial state
// (so it can be reconnected if necessary).
func (d *DbusConnManager) resetConnection(conn *systemdDbus.Conn) {
	dbusMu.Lock()
	defer dbusMu.Unlock()
	if dbusC != nil && dbusC == conn {
		dbusC.Close()
		dbusC = nil
	}
}
