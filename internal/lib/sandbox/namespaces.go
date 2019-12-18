package sandbox

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	NETNS = "net"
	IPCNS = "ipc"
	UTSNS = "uts"
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
	if len(managedNamespaces) == 0 {
		return make(map[string]string), nil
	}

	namespaces, err := PinManagedNamespaces(managedNamespaces, pinnsPath)
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

// NetNs specific functions

// NetNs retrieves the network namespace of the sandbox
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) NetNs() *Namespace {
	if s.netns == nil {
		return nil
	}
	return s.netns.Get()
}

// NetNsSet sets the sandbox's netns
func (s *Sandbox) NetNsSet(net NamespaceIface) {
	s.netns = net
}

// NetNsPath returns the path to the network namespace of the sandbox.
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) NetNsPath() string {
	return s.nsPath(s.netns, NETNS)
}

// NetNsJoin attempts to join the sandbox to an existing network namespace
// This will fail if the sandbox is already part of a network namespace
func (s *Sandbox) NetNsJoin(nspath string) error {
	if s.netns != nil {
		return fmt.Errorf("sandbox already has a network namespace, cannot join another")
	}

	netNS, err := getNamespace(nspath)
	if err != nil {
		return err
	}

	s.netns = netNS

	return nil
}

// IpcNs specific functions

// IpcNs retrieves the IPC namespace of the sandbox
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) IpcNs() *Namespace {
	if s.ipcns == nil {
		return nil
	}
	return s.ipcns.Get()
}

// IpcNsSet sets the sandbox's ipcns
func (s *Sandbox) IpcNsSet(ipc NamespaceIface) {
	s.ipcns = ipc
}

// IpcNsPath returns the path to the network namespace of the sandbox.
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) IpcNsPath() string {
	return s.nsPath(s.ipcns, IPCNS)
}

// IpcNsJoin attempts to join the sandbox to an existing IPC namespace
// This will fail if the sandbox is already part of a IPC namespace
func (s *Sandbox) IpcNsJoin(nspath string) error {
	if s.ipcns != nil {
		return fmt.Errorf("sandbox already has a ipc namespace, cannot join another")
	}

	ipcNS, err := getNamespace(nspath)
	if err != nil {
		return err
	}

	s.ipcns = ipcNS

	return nil
}

// UtsNs specific functions

// UtsNs retrieves the UTS namespace of the sandbox
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) UtsNs() *Namespace {
	if s.utsns == nil {
		return nil
	}
	return s.utsns.Get()
}

// UtsNsSet sets the sandbox's utsns
func (s *Sandbox) UtsNsSet(uts NamespaceIface) {
	s.utsns = uts
}

// UtsNsPath returns the path to the network namespace of the sandbox.
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) UtsNsPath() string {
	return s.nsPath(s.utsns, UTSNS)
}

// UtsNsJoin attempts to join the sandbox to an existing UTS namespace
// This will fail if the sandbox is already part of a UTS namespace
func (s *Sandbox) UtsNsJoin(nspath string) error {
	if s.utsns != nil {
		return fmt.Errorf("sandbox already has a uts namespace, cannot join another")
	}

	utsNS, err := getNamespace(nspath)
	if err != nil {
		return err
	}

	s.utsns = utsNS

	return nil
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

// nsPath returns the path to a namespace of the sandbox.
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) nsPath(ns NamespaceIface, nsType string) string {
	if ns == nil || ns.Get() == nil {
		if s.infraContainer != nil {
			return fmt.Sprintf("/proc/%v/ns/%s", s.infraContainer.State().Pid, nsType)
		}
		return ""
	}

	return ns.Path()
}

// UserNsPath returns the path to the user namespace of the sandbox.
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) UserNsPath() string {
	if s.infraContainer != nil {
		return fmt.Sprintf("/proc/%v/ns/user", s.infraContainer.State().Pid)
	}
	return ""
}
