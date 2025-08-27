package server

import (
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
var defaultLinuxMaskedPaths = sync.OnceValue(func() []string {
	maskedPaths := slices.Concat(
		config.DefaultMaskedPaths(),
		[]string{"/proc/asound", "/proc/interrupts"},
	)

	return maskedPaths
})
