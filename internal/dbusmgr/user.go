// Code in this package is heavily adapted from https://github.com/opencontainers/runc/blob/7362fa2d282feffb9b19911150e01e390a23899d/libcontainer/cgroups/systemd
// Credit goes to the runc authors.
package dbusmgr

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	systemdDbus "github.com/coreos/go-systemd/v22/dbus"
	dbus "github.com/godbus/dbus/v5"
	"github.com/moby/sys/userns"

	"github.com/cri-o/cri-o/utils/cmdrunner"
)

// newUserSystemdDbus creates a connection for systemd user-instance.
func newUserSystemdDbus() (*systemdDbus.Conn, error) {
	addr, err := DetectUserDbusSessionBusAddress()
	if err != nil {
		return nil, err
	}
	uid, err := DetectUID()
	if err != nil {
		return nil, err
	}

	return systemdDbus.NewConnection(func() (*dbus.Conn, error) {
		conn, err := dbus.Dial(addr)
		if err != nil {
			return nil, fmt.Errorf("error while dialing %q: %w", addr, err)
		}
		methods := []dbus.Auth{dbus.AuthExternal(strconv.Itoa(uid))}
		err = conn.Auth(methods)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("error while authenticating connection, address=%q, UID=%d: %w", addr, uid, err)
		}
		if err = conn.Hello(); err != nil {
			conn.Close()
			return nil, fmt.Errorf("error while sending Hello message, address=%q, UID=%d: %w", addr, uid, err)
		}
		return conn, nil
	})
}

// DetectUID detects UID from the OwnerUID field of `busctl --user status`
// if running in userNS. The value corresponds to sd_bus_creds_get_owner_uid(3) .
//
// Otherwise returns os.Getuid() .
func DetectUID() (int, error) {
	if !userns.RunningInUserNS() {
		return os.Getuid(), nil
	}
	b, err := cmdrunner.Command("busctl", "--user", "--no-pager", "status").CombinedOutput()
	if err != nil {
		return -1, fmt.Errorf("could not execute `busctl --user --no-pager status`: %q: %w", string(b), err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		s := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(s, "OwnerUID=") {
			uidStr := strings.TrimPrefix(s, "OwnerUID=")
			i, err := strconv.Atoi(uidStr)
			if err != nil {
				return -1, fmt.Errorf("could not detect the OwnerUID: %s: %w", s, err)
			}
			return i, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return -1, err
	}
	return -1, errors.New("could not detect the OwnerUID")
}

// DetectUserDbusSessionBusAddress returns $DBUS_SESSION_BUS_ADDRESS if set.
// Otherwise returns "unix:path=$XDG_RUNTIME_DIR/bus" if $XDG_RUNTIME_DIR/bus exists.
// Otherwise parses the value from `systemctl --user show-environment` .
func DetectUserDbusSessionBusAddress() (string, error) {
	if env := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); env != "" {
		return env, nil
	}
	if xdr := os.Getenv("XDG_RUNTIME_DIR"); xdr != "" {
		busPath := filepath.Join(xdr, "bus")
		if _, err := os.Stat(busPath); err == nil {
			busAddress := "unix:path=" + busPath
			return busAddress, nil
		}
	}
	b, err := cmdrunner.Command("systemctl", "--user", "--no-pager", "show-environment").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("could not execute `systemctl --user --no-pager show-environment`, output=%q: %w", string(b), err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		s := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(s, "DBUS_SESSION_BUS_ADDRESS=") {
			return strings.TrimPrefix(s, "DBUS_SESSION_BUS_ADDRESS="), nil
		}
	}
	return "", errors.New("could not detect DBUS_SESSION_BUS_ADDRESS from `systemctl --user --no-pager show-environment`. Make sure you have installed the dbus-user-session or dbus-daemon package and then run: `systemctl --user start dbus`")
}
