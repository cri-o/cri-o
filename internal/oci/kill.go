package oci

import (
	"syscall"

	"github.com/docker/docker/pkg/signal"
	"github.com/pkg/errors"
)

// Reverse lookup signal string from its map
func findStringInSignalMap(killSignal syscall.Signal) (string, error) {
	for k, v := range signal.SignalMap {
		if v == killSignal {
			return k, nil
		}
	}
	return "", errors.Errorf("unable to convert signal to string")
}
