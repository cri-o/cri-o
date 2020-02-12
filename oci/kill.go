package oci

import (
	"syscall"

	"github.com/docker/docker/pkg/signal"
)

// Check if killSignal exists in the signal map
func inSignalMap(killSignal syscall.Signal) bool {
	for _, v := range signal.SignalMap {
		if v == killSignal {
			return true
		}
	}
	return false

}
