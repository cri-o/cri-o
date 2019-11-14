package sandbox

import (
	"errors"
	"fmt"
	"os"

	nspkg "github.com/containernetworking/plugins/pkg/ns"
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
	Initialize(nsType string) (NamespaceIface, error)

	// Initialized returns true if already initialized
	Initialized() bool

	// Remove ensures this network namespace handle is closed and removed
	Remove() error

	// SymlinkCreate creates all necessary symlinks
	SymlinkCreate(string) error
}


func (s *Sandbox) CreateSandboxNamespaces(managedNamespaces []string) (map[string]int, error) {
	namespaces, err := createNewNamespaces(managedNamespaces)
	if err != nil {
		return nil, err
	}

	namespaceIfaces := make([]NamespaceIface, 0)
	for _, namespace := range namespaces {
		namespaceIface, err := namespace.Initialize(namespace.nsType)
		if err != nil {
			return nil, err
		}
		namespaceIfaces = append(namespaceIfaces, namespaceIface)
	}
	return nil, nil
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

// NetNsPath returns the path to the network namespace of the sandbox.
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) NetNsPath() string {
	return s.nsPath(s.netns, NETNS)
}

// NetNsCreate creates a new network namespace for the sandbox
func (s *Sandbox) NetNsCreate(netNs NamespaceIface) error {
	netNs, err := s.nsCreate(netNs, NETNS)
	if err != nil {
		return err
	}
	s.netns = netNs
	return nil
}

// NetNsJoin attempts to join the sandbox to an existing network namespace
// This will fail if the sandbox is already part of a network namespace
func (s *Sandbox) NetNsJoin(nspath, name string) error {
	if s.netns != nil {
		return fmt.Errorf("sandbox already has a network namespace, cannot join another")
	}

	netNS, err := s.NsGet(nspath, name)
	if err != nil {
		return err
	}

	s.netns = netNS

	return nil
}

// NetNsRemove removes the network namespace associated with the sandbox
func (s *Sandbox) NetNsRemove() error {
	if s.netns == nil {
		logrus.Warn("no networking namespace")
		return nil
	}

	return s.netns.Remove()
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

// IpcNsPath returns the path to the network namespace of the sandbox.
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) IpcNsPath() string {
	return s.nsPath(s.ipcns, IPCNS)
}

// IpcNsCreate creates a new IPC namespace for the sandbox
func (s *Sandbox) IpcNsCreate(ipcNs NamespaceIface) error {
	ipcNs, err := s.nsCreate(ipcNs, IPCNS)
	if err != nil {
		return err
	}
	s.ipcns = ipcNs
	return nil
}

// IpcNsJoin attempts to join the sandbox to an existing IPC namespace
// This will fail if the sandbox is already part of a IPC namespace
func (s *Sandbox) IpcNsJoin(nspath, name string) error {
	if s.ipcns != nil {
		return fmt.Errorf("sandbox already has a ipc namespace, cannot join another")
	}

	ipcNS, err := s.NsGet(nspath, name)
	if err != nil {
		return err
	}

	s.ipcns = ipcNS

	return nil
}

// IpcNsRemove removes the IPC namespace associated with the sandbox
func (s *Sandbox) IpcNsRemove() error {
	if s.ipcns == nil {
		logrus.Warn("no IPC namespace")
		return nil
	}

	return s.ipcns.Remove()
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

// UtsNsPath returns the path to the network namespace of the sandbox.
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) UtsNsPath() string {
	return s.nsPath(s.utsns, UTSNS)
}

// UtsNsCreate creates a new UTS namespace for the sandbox
func (s *Sandbox) UtsNsCreate(utsNs NamespaceIface) error {
	utsNs, err := s.nsCreate(utsNs, UTSNS)
	if err != nil {
		return err
	}
	s.utsns = utsNs
	return nil
}

// UtsNsJoin attempts to join the sandbox to an existing UTS namespace
// This will fail if the sandbox is already part of a UTS namespace
func (s *Sandbox) UtsNsJoin(nspath, name string) error {
	if s.utsns != nil {
		return fmt.Errorf("sandbox already has a uts namespace, cannot join another")
	}

	utsNS, err := s.NsGet(nspath, name)
	if err != nil {
		return err
	}

	s.utsns = utsNS

	return nil
}

// UtsNsRemove removes the UTS namespace associated with the sandbox
func (s *Sandbox) UtsNsRemove() error {
	if s.utsns == nil {
		logrus.Warn("no UTS namespace")
		return nil
	}

	return s.utsns.Remove()
}

// nsPath returns the path to a namespace of the sandbox.
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) nsPath(ns NamespaceIface, nsType string) string {
	if ns == nil || ns.Get() == nil ||
		ns.Get().symlink == nil {
		if s.infraContainer != nil {
			return fmt.Sprintf("/proc/%v/ns/%s", s.infraContainer.State().Pid, nsType)
		}
		return ""
	}

	return ns.Get().symlink.Name()
}

// nsCreate creates a new namespace of type nsType for the sandbox
func (s *Sandbox) nsCreate(nsIface NamespaceIface, nsType string) (NamespaceIface, error) {
	// Create a new netNs if nil provided
	if nsIface == nil {
		nsIface = &Namespace{}
	}

	// Check if interface is already initialized
	if nsIface.Initialized() {
		return nsIface, fmt.Errorf("%s NS already initialized", nsType)
	}

	nsIface, err := nsIface.Initialize(nsType)
	if err != nil {
		return nsIface, err
	}

	if err := nsIface.SymlinkCreate(s.name); err != nil {
		logrus.Warnf("Could not create %sns symlink %v", nsType, err)

		if err1 := nsIface.Close(); err1 != nil {
			return nsIface, err1
		}

		return nsIface, err
	}
	return nsIface, nil
}

// NsGet returns the Namespace associated with the given nspath and name
func (s *Sandbox) NsGet(nspath, name string) (*Namespace, error) {
	if err := nspkg.IsNSorErr(nspath); err != nil {
		return nil, ErrClosedNS
	}

	symlink, symlinkErr := isSymbolicLink(nspath)
	if symlinkErr != nil {
		return nil, symlinkErr
	}

	var resolvedNsPath string
	if symlink {
		path, err := os.Readlink(nspath)
		if err != nil {
			return nil, err
		}
		resolvedNsPath = path
	} else {
		resolvedNsPath = nspath
	}

	ns, err := getNamespace(resolvedNsPath)
	if err != nil {
		return nil, err
	}

	if symlink {
		fd, err := os.Open(nspath)
		if err != nil {
			return nil, err
		}

		ns.symlink = fd
	} else if err := ns.SymlinkCreate(name); err != nil {
		return nil, err
	}

	return ns, nil
}

func isSymbolicLink(path string) (bool, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return false, err
	}

	return fi.Mode()&os.ModeSymlink == os.ModeSymlink, nil
}

// UserNsPath returns the path to the user namespace of the sandbox.
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) UserNsPath() string {
	if s.infraContainer != nil {
		return fmt.Sprintf("/proc/%v/ns/user", s.infraContainer.State().Pid)
	}
	return ""
}
