//go:build !windows
// +build !windows

package signals

import (
	"os"

	"golang.org/x/sys/unix"
)

// Platform specific signal synonyms
var (
	Term os.Signal = unix.SIGTERM
	Hup  os.Signal = unix.SIGHUP
)
