package signals

import "os"

// Cross platform signal synonyms.
var (
	Interrupt = os.Interrupt
	Kill      = os.Kill
)
