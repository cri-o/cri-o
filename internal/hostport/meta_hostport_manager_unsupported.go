//go:build !linux

package hostport

import (
	"context"
	"fmt"
	"runtime"
)

// NewMetaHostportManager creates a new HostPortManager. It is only supported
// on Linux, because sigs.k8s.io/knftables does not build on other platforms.
func NewMetaHostportManager(context.Context) (HostPortManager, error) {
	return nil, fmt.Errorf("can't create HostPortManager: unsupported on %s", runtime.GOOS)
}
