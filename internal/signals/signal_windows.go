package signals

import (
	"os"

	"golang.org/x/sys/windows"
)

// Platform specific signal synonyms
var (
	Term os.Signal = windows.SIGTERM
	Hup  os.Signal = windows.SIGHUP
)
