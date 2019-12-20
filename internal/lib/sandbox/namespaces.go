package sandbox

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	NETNS  = "net"
	IPCNS  = "ipc"
	UTSNS  = "uts"
	USERNS = "user"
)

// ErrClosedNS is the error returned when the network namespace of the
// sandbox is closed
var ErrClosedNS = errors.New("PodSandbox namespace is closed")

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
	Type() string
}

func (s *Sandbox) CreateManagedNamespaces(managedNamespaces []string, pinnsPath string) (map[string]string, error) {
	return s.CreateNamespacesWithFunc(managedNamespaces, pinnsPath, pinNamespaces)
}

func (s *Sandbox) CreateNamespacesWithFunc(managedNamespaces []string, pinnsPath string, pinFunc func([]string, string) ([]NamespaceIface, error)) (map[string]string, error) {
	if len(managedNamespaces) == 0 {
		return make(map[string]string), nil
	}

	namespaces, err := pinFunc(managedNamespaces, pinnsPath)
	if err != nil {
		return nil, err
	}

	nsTypeToPath := make(map[string]string)

	for _, namespace := range namespaces {
		namespaceIface := namespace.Initialize()
		defer func() {
			if err != nil {
				if err1 := namespaceIface.Remove(); err1 != nil {
					logrus.Warnf("removing namespace interface returned: %v", err1)
				}
			}
		}()

		switch namespace.Type() {
		case NETNS:
			s.netns = namespaceIface
			nsTypeToPath[NETNS] = namespace.Path()
		case IPCNS:
			s.ipcns = namespaceIface
			nsTypeToPath[IPCNS] = namespace.Path()
		case UTSNS:
			s.utsns = namespaceIface
			nsTypeToPath[UTSNS] = namespace.Path()
		default:
			// This should never happen
			err = errors.New("Invalid namespace type")
			return nil, err
		}
	}

	return nsTypeToPath, nil
}

// NamespacePaths returns all the paths of the
// namespaces of the sandbox. If a namespace is not
// managed by the sandbox, the namespace of the infra
// container will be returned.
// It returns a map of nsType -> path, allowing for
// callers to branch on the namespace type
func (s *Sandbox) NamespacePaths() map[string]string {
	pid := infraPid(s.InfraContainer())

	paths := make(map[string]string)

	if ipc := nsPathGivenInfraPid(s.ipcns, IPCNS, pid); ipc != "" {
		paths[IPCNS] = ipc
	}
	if net := nsPathGivenInfraPid(s.netns, NETNS, pid); net != "" {
		paths[NETNS] = net
	}
	if uts := nsPathGivenInfraPid(s.utsns, UTSNS, pid); uts != "" {
		paths[UTSNS] = uts
	}
	return paths
}

// RemoveManagedNamespaces cleans up after managing the namespaces. It removes all of the namespaces
// and the parent directory in which they lived.
func (s *Sandbox) RemoveManagedNamespaces() error {
	errs := make([]error, 0)

	// use a map as a set to delete each parent directory just once
	directories := make(map[string]bool)
	if s.utsns != nil {
		directories[filepath.Dir(s.utsns.Path())] = true
		if err := s.utsns.Remove(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.ipcns != nil {
		directories[filepath.Dir(s.ipcns.Path())] = true
		if err := s.ipcns.Remove(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.netns != nil {
		directories[filepath.Dir(s.netns.Path())] = true
		if err := s.netns.Remove(); err != nil {
			errs = append(errs, err)
		}
	}

	for directory := range directories {
		if err := os.RemoveAll(directory); err != nil {
			errs = append(errs, err)
		}
	}
	var err error
	if len(errs) != 0 {
		err = errors.Errorf("Removing namespaces encountered the following errors %v", errs)
	}
	return err
}

// NetNs specific functions

// NetNsPath returns the path to the network namespace of the sandbox.
// If the sandbox uses the host namespace, the empty string is returned
func (s *Sandbox) NetNsPath() string {
	return s.nsPath(s.netns, NETNS)
}

// NetNsJoin attempts to join the sandbox to an existing network namespace
// This will fail if the sandbox is already part of a network namespace
func (s *Sandbox) NetNsJoin(nspath string) (err error) {
	s.netns, err = nsJoin(nspath, NETNS, s.netns)
	return err
}

// IpcNs specific functions

// IpcNsPath returns the path to the network namespace of the sandbox.
// If the sandbox uses the host namespace, the empty string is returned
func (s *Sandbox) IpcNsPath() string {
	return s.nsPath(s.ipcns, IPCNS)
}

// IpcNsJoin attempts to join the sandbox to an existing IPC namespace
// This will fail if the sandbox is already part of a IPC namespace
func (s *Sandbox) IpcNsJoin(nspath string) (err error) {
	s.ipcns, err = nsJoin(nspath, IPCNS, s.ipcns)
	return err
}

// UtsNs specific functions

// UtsNsPath returns the path to the network namespace of the sandbox.
// If the sandbox uses the host namespace, the empty string is returned
func (s *Sandbox) UtsNsPath() string {
	return s.nsPath(s.utsns, UTSNS)
}

// UtsNsJoin attempts to join the sandbox to an existing UTS namespace
// This will fail if the sandbox is already part of a UTS namespace
func (s *Sandbox) UtsNsJoin(nspath string) (err error) {
	s.utsns, err = nsJoin(nspath, UTSNS, s.utsns)
	return err
}

// UserNs specific functions

// UserNsPath returns the path to the user namespace of the sandbox.
// If the sandbox uses the host namespace, the empty string is returned
func (s *Sandbox) UserNsPath() string {
	return s.nsPath(nil, USERNS)
}

// nsJoin checks if the current iface is nil, and if so gets the namespace at nsPath
func nsJoin(nsPath, nsType string, currentIface NamespaceIface) (NamespaceIface, error) {
	if currentIface != nil {
		return currentIface, fmt.Errorf("sandbox already has a %s namespace, cannot join another", nsType)
	}

	return getNamespace(nsPath)
}

// nsPath returns the path to a namespace of the sandbox.
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) nsPath(ns NamespaceIface, nsType string) string {
	return nsPathGivenInfraPid(ns, nsType, infraPid(s.InfraContainer()))
}

// infraPid returns the pid of the passed in infra container
// if the infra container is nil, pid is returned negative
func infraPid(infra *oci.Container) int {
	pid := -1
	if infra != nil {
		pid = infra.State().Pid
	}
	return pid
}

// nsPathGivenInfraPid allows callers to cache the infra pid, rather than
// calling a container.State() in batch operations
func nsPathGivenInfraPid(ns NamespaceIface, nsType string, infraPid int) string {
	// caller is responsible for checking if infraContainer
	// is valid. If not, infraPid should be negative
	if ns == nil || ns.Get() == nil {
		if infraPid >= 0 {
			return infraNsPath(nsType, infraPid)
		}
		return ""
	}

	return ns.Path()
}

// infraNsPath returns the namespace path of type nsType for infra
// with pid infraContainerPid
func infraNsPath(nsType string, infraContainerPid int) string {
	return fmt.Sprintf("/proc/%d/ns/%s", infraContainerPid, nsType)
}
