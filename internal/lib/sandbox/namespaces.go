package sandbox

import (
	"fmt"
	"os"

	"github.com/cri-o/cri-o/internal/config/nsmgr"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ManagedNamespace is a structure that holds all the necessary information a caller would
// need for a sandbox managed namespace
// Where nsmgr.Namespace does hold similar information, ManagedNamespace exists to allow this library
// to not return data not necessarily in a Namespace (for instance, when a namespace is not managed
// by CRI-O, but instead is based off of the infra pid)
type ManagedNamespace struct {
	nsPath string
	nsType nsmgr.NSType
}

// Type returns the namespace type
func (m *ManagedNamespace) Type() nsmgr.NSType {
	return m.nsType
}

// Type returns the namespace path
func (m *ManagedNamespace) Path() string {
	return m.nsPath
}

func (s *Sandbox) AddManagedNamespaces(namespaces []nsmgr.Namespace) {
	// if the namespace structure wasn't initialized, we have nothing to do here
	if namespaces == nil {
		return
	}
	for _, ns := range namespaces {
		// skip any nil entries
		if ns == nil {
			continue
		}
		switch ns.Type() {
		case nsmgr.IPCNS:
			s.ipcns = ns
		case nsmgr.UTSNS:
			s.utsns = ns
		case nsmgr.NETNS:
			s.netns = ns
		case nsmgr.USERNS:
			s.userns = ns
		default:
			// this should never happen, as we control the NSTypes
			panic(errors.Errorf("unknown namespace type %s", ns))
		}
	}
}

// NamespacePaths returns all the paths of the namespaces of the sandbox. If a namespace is not
// managed by the sandbox, the namespace of the infra container will be returned.
// It returns a slice of ManagedNamespaces
func (s *Sandbox) NamespacePaths() []*ManagedNamespace {
	pid := infraPid(s.InfraContainer())

	typesAndPaths := make([]*ManagedNamespace, 0, nsmgr.ManagedNamespacesNum)

	if ipc := nsPathGivenInfraPid(s.ipcns, nsmgr.IPCNS, pid); ipc != "" {
		typesAndPaths = append(typesAndPaths, &ManagedNamespace{
			nsType: nsmgr.IPCNS,
			nsPath: ipc,
		})
	}
	if net := nsPathGivenInfraPid(s.netns, nsmgr.NETNS, pid); net != "" {
		typesAndPaths = append(typesAndPaths, &ManagedNamespace{
			nsType: nsmgr.NETNS,
			nsPath: net,
		})
	}
	if uts := nsPathGivenInfraPid(s.utsns, nsmgr.UTSNS, pid); uts != "" {
		typesAndPaths = append(typesAndPaths, &ManagedNamespace{
			nsType: nsmgr.UTSNS,
			nsPath: uts,
		})
	}
	if user := nsPathGivenInfraPid(s.userns, nsmgr.USERNS, pid); user != "" {
		typesAndPaths = append(typesAndPaths, &ManagedNamespace{
			nsType: nsmgr.USERNS,
			nsPath: user,
		})
	}
	return typesAndPaths
}

// RemoveManagedNamespaces cleans up after managing the namespaces. It removes all of the namespaces
// and the parent directory in which they lived.
func (s *Sandbox) RemoveManagedNamespaces() error {
	errs := make([]error, 0)

	// use a map as a set to delete each parent directory just once
	if s.utsns != nil {
		if err := s.utsns.Remove(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.ipcns != nil {
		if err := s.ipcns.Remove(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.netns != nil {
		if err := s.netns.Remove(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.userns != nil {
		if err := s.userns.Remove(); err != nil {
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
	return s.nsPath(s.netns, nsmgr.NETNS)
}

// NetNsJoin attempts to join the sandbox to an existing network namespace
// This will fail if the sandbox is already part of a network namespace
func (s *Sandbox) NetNsJoin(nspath string) error {
	ns, err := nsJoin(nspath, nsmgr.NETNS, s.netns)
	if err != nil {
		return err
	}
	s.netns = ns
	return nil
}

// IpcNs specific functions

// IpcNsPath returns the path to the network namespace of the sandbox.
// If the sandbox uses the host namespace, the empty string is returned
func (s *Sandbox) IpcNsPath() string {
	return s.nsPath(s.ipcns, nsmgr.IPCNS)
}

// IpcNsJoin attempts to join the sandbox to an existing IPC namespace
// This will fail if the sandbox is already part of a IPC namespace
func (s *Sandbox) IpcNsJoin(nspath string) error {
	ns, err := nsJoin(nspath, nsmgr.IPCNS, s.ipcns)
	if err != nil {
		return err
	}
	s.ipcns = ns
	return err
}

// UtsNs specific functions

// UtsNsPath returns the path to the network namespace of the sandbox.
// If the sandbox uses the host namespace, the empty string is returned
func (s *Sandbox) UtsNsPath() string {
	return s.nsPath(s.utsns, nsmgr.UTSNS)
}

// UtsNsJoin attempts to join the sandbox to an existing UTS namespace
// This will fail if the sandbox is already part of a UTS namespace
func (s *Sandbox) UtsNsJoin(nspath string) error {
	ns, err := nsJoin(nspath, nsmgr.UTSNS, s.utsns)
	if err != nil {
		return err
	}
	s.utsns = ns
	return err
}

// UserNs specific functions

// UserNsPath returns the path to the user namespace of the sandbox.
// If the sandbox uses the host namespace, the empty string is returned
func (s *Sandbox) UserNsPath() string {
	return s.nsPath(s.userns, nsmgr.USERNS)
}

// UserNsJoin attempts to join the sandbox to an existing User namespace
// This will fail if the sandbox is already part of a User namespace
func (s *Sandbox) UserNsJoin(nspath string) error {
	ns, err := nsJoin(nspath, nsmgr.USERNS, s.userns)
	if err != nil {
		return err
	}
	s.userns = ns
	return err
}

// PidNs specific functions

// PidNsPath returns the path to the pid namespace of the sandbox.
// If the sandbox uses the host namespace, the empty string is returned.
func (s *Sandbox) PidNsPath() string {
	return s.nsPath(nil, nsmgr.PIDNS)
}

// nsJoin checks if the current iface is nil, and if so gets the namespace at nsPath
func nsJoin(nsPath string, nsType nsmgr.NSType, currentIface nsmgr.Namespace) (nsmgr.Namespace, error) {
	if currentIface != nil {
		return currentIface, fmt.Errorf("sandbox already has a %s namespace, cannot join another", nsType)
	}

	return nsmgr.GetNamespace(nsPath, nsType)
}

// nsPath returns the path to a namespace of the sandbox.
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) nsPath(ns nsmgr.Namespace, nsType nsmgr.NSType) string {
	return nsPathGivenInfraPid(ns, nsType, infraPid(s.InfraContainer()))
}

// infraPid returns the pid of the passed in infra container
// if the infra container is nil, pid is returned negative
func infraPid(infra *oci.Container) int {
	pid := -1
	if infra != nil && !infra.Spoofed() {
		var err error
		pid, err = infra.Pid()
		// There are some cases where ErrNotInitialized is expected.
		// For instance, when we're creating a pod sandbox while managing namespace lifecycle,
		// we create the network stack before we create the infra container.
		// Since we're pinning namespaces, we already have the namespace we need.
		// Later users of this pid will either find we have a valid pinned namespace (which we will in this case),
		// or find we have an invalid /proc entry (a negative pid).
		// Thus, we don't need to error here if the pid is not initialized
		if err != nil && err != oci.ErrNotInitialized {
			logrus.Errorf("Pid for infra container %s not found: %v", infra.ID(), err)
		}
	}
	return pid
}

// nsPathGivenInfraPid allows callers to cache the infra pid, rather than
// calling a container.State() in batch operations
func nsPathGivenInfraPid(ns nsmgr.Namespace, nsType nsmgr.NSType, infraPid int) string {
	// caller is responsible for checking if infraContainer
	// is valid. If not, infraPid should be less than or equal to 0
	if ns == nil {
		if infraPid > 0 {
			return infraNsPath(nsType, infraPid)
		}
		return ""
	}

	return ns.Path()
}

// infraNsPath returns the namespace path of type nsType for infra
// with pid infraContainerPid
func infraNsPath(nsType nsmgr.NSType, infraContainerPid int) string {
	// verify nsPath exists on the host. This will prevent us from fatally erroring
	// on network tear down if the path doesn't exist
	// Technically, this is pretty racy, but so is every check using the infra container PID.
	// Without managing the namespaces, this is the best we can do
	nsPath := fmt.Sprintf("/proc/%d/ns/%s", infraContainerPid, nsType)
	if _, err := os.Stat(nsPath); err != nil {
		return ""
	}
	return nsPath
}
