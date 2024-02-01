//go:build !linux && !freebsd
// +build !linux,!freebsd

package nsmgr

import (
	"fmt"
	"runtime"
)

// NamespaceManager manages the server's namespaces.
// Specifically, it is an interface for how the server is creating namespaces,
// and can be requested to create namespaces for a pod.
type NamespaceManager struct {
}

// New creates a new NamespaceManager.
func New(namespacesDir, pinnsPath string) *NamespaceManager {
	return &NamespaceManager{}
}

func (mgr *NamespaceManager) Initialize() error {
	return nil
}

// GetNamespace takes a path and type, checks if it is a namespace, and if so
// returns an instance of the Namespace interface.
func GetNamespace(_ string, _ NSType) (Namespace, error) {
	return nil, fmt.Errorf("GetNamespace not supported on %s", runtime.GOOS)
}

// NamespacePathFromProc returns the namespace path of type nsType for a given pid and type.
func NamespacePathFromProc(nsType NSType, pid int) string {
	return ""
}

func (mgr *NamespaceManager) NamespaceFromProcEntry(pid int, nsType NSType) (_ Namespace, retErr error) {
	return nil, fmt.Errorf("(*NamespaceManager).NamespaceFromProcEntry unsupported on %s", runtime.GOOS)
}
