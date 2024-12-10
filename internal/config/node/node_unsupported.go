//go:build !linux && !freebsd

package node

import (
	"fmt"
	"runtime"
)

func ValidateConfig() error {
	return fmt.Errorf("CRI-O is not supported on %s", runtime.GOOS)
}
