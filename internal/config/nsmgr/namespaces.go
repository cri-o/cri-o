// +build linux

package nsmgr

import (
	"github.com/containers/storage/pkg/idtools"
)

// NSType is an abstraction about available namespace types
type NSType string

const (
	NETNS                NSType = "net"
	IPCNS                NSType = "ipc"
	UTSNS                NSType = "uts"
	USERNS               NSType = "user"
	PIDNS                NSType = "pid"
	ManagedNamespacesNum        = 4
)

// NamespaceIface provides a generic namespace interface
type NamespaceIface interface {
	// Close closes this network namespace
	Close() error

	// Get returns the native Namespace
	Get() *Namespace

	// Initialize does the necessary setup
	Initialize() NamespaceIface

	// Initialized returns true if already initialized
	Initialized() bool

	// Remove ensures this network namespace handle is closed and removed
	Remove() error

	// Path returns the bind mount path of the namespace
	Path() string

	// Type returns the namespace type (net, ipc or uts)
	Type() NSType
}

type namespacePinner func([]NSType, *idtools.IDMappings, map[string]string) ([]NamespaceIface, error)

func (mgr *managedNamespaceManager) NewPodNamespaces(managedNamespaces []NSType, idMappings *idtools.IDMappings, sysctls map[string]string) ([]NamespaceIface, error) {
	return mgr.NewPodNamespacesWithFunc(managedNamespaces, idMappings, sysctls, mgr.pinNamespaces)
}

// CreateManagedNamespacesWithFunc is mainly added for testing purposes. There's no point in actually calling the pinns binary
// in unit tests, so this function allows the actual pin func to be abstracted out. Every other caller should use CreateManagedNamespaces
func (mgr *managedNamespaceManager) NewPodNamespacesWithFunc(managedNamespaces []NSType, idMappings *idtools.IDMappings, sysctls map[string]string, pinFunc namespacePinner) (mns []NamespaceIface, retErr error) {
	if len(managedNamespaces) == 0 {
		return []NamespaceIface{}, nil
	}

	return pinFunc(managedNamespaces, idMappings, sysctls)
}
