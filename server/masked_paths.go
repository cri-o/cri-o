package server

import (
	"os"
	"slices"
	"sync"

	"github.com/containers/common/pkg/config"
)

// appendDefaultMaskedPaths is retrieving the default masked paths and appends
// the existing ones to it.
func appendDefaultMaskedPaths(additionalPaths []string) []string {
	paths := slices.Concat(defaultLinuxMaskedPaths(), additionalPaths)
	slices.Sort(paths)

	return slices.Compact(paths)
}

// defaultLinuxMaskedPaths will be used to evaluate the default masked paths once.
func defaultLinuxMaskedPaths() []string {
	maskedPaths := defaultLinuxMaskedPathsWithoutThermalThrottle()
	slices.DeleteFunc(maskedPaths, func(path string) bool {
		_, err := os.Stat(path)
		return err != nil
	})
	return maskedPaths
}

var defaultLinuxMaskedPathsWithoutThermalThrottle = sync.OnceValue(func() []string {
	maskedPaths := slices.Concat(
		config.DefaultMaskedPaths(),
		[]string{"/proc/asound", "/proc/interrupts"},
	)
	return maskedPaths
})
