// +build linux

package nsmgr

import (
	"os"
	"sync"

	nspkg "github.com/containernetworking/plugins/pkg/ns"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
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

// Namespace provides a generic namespace interface
type Namespace interface {
	// Remove ensures this network namespace handle is closed and removed
	Remove() error

	// Path returns the bind mount path of the namespace
	Path() string

	// Type returns the namespace type (net, ipc or uts)
	Type() NSType
}

// namespace handles data pertaining to a namespace
type namespace struct {
	sync.Mutex
	ns     NS
	closed bool
	nsType NSType
	nsPath string
}

// NS is a wrapper for the containernetworking plugin's NetNS interface
// It exists because while NetNS is specifically called such, it is really a generic
// namespace, and can be used for other namespaces
type NS interface {
	nspkg.NetNS
}

// Path returns the path of the namespace handle
func (n *namespace) Path() string {
	if n == nil || n.ns == nil {
		return ""
	}
	return n.nsPath
}

// Type returns which namespace this structure represents
func (n *namespace) Type() NSType {
	return n.nsType
}

// Remove ensures this namespace handle is closed and removed
func (n *namespace) Remove() error {
	n.Lock()
	defer n.Unlock()

	if n.closed {
		// nsRemove() can be called multiple
		// times without returning an error.
		return nil
	}

	if err := n.ns.Close(); err != nil {
		return err
	}

	n.closed = true

	fp := n.Path()
	if fp == "" {
		return nil
	}

	// try to unmount, ignoring "not mounted" (EINVAL) error
	if err := unix.Unmount(fp, unix.MNT_DETACH); err != nil && err != unix.EINVAL {
		return errors.Wrapf(err, "unable to unmount %s", fp)
	}
	return os.RemoveAll(fp)
}

// GetNamespace takes a path and a type, checks if it is a namespace, and if so
// returns a Namespace
func GetNamespace(nsPath string, nsType NSType) (Namespace, error) {
	if err := nspkg.IsNSorErr(nsPath); err != nil {
		return nil, err
	}

	ns, err := nspkg.GetNS(nsPath)
	if err != nil {
		return nil, err
	}

	return &namespace{ns: ns, nsType: nsType, nsPath: nsPath}, nil
}
