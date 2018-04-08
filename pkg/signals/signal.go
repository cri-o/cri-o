package signals

import "os"

// Cross platform signal synonyms
var (
	Interrupt os.Signal = os.Interrupt
	Kill      os.Signal = os.Kill
)
