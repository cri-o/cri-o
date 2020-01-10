// +build linux

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	nspkg "github.com/containernetworking/plugins/pkg/ns"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/docker/docker/pkg/mount"
	"github.com/docker/docker/pkg/symlink"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// Namespace handles data pertaining to a namespace
type Namespace struct {
	sync.Mutex
	ns          NS
	closed      bool
	initialized bool
	nsType      NSType
	nsPath      string
}

// NS is a wrapper for the containernetworking plugin's NetNS interface
// It exists because while NetNS is specifically called such, it is really a generic
// namespace, and can be used for other namespaces
type NS interface {
	nspkg.NetNS
}

// Get returns the Namespace for a given NsIface
func (n *Namespace) Get() *Namespace {
	return n
}

// Initialized returns true if the Namespace is already initialized
func (n *Namespace) Initialized() bool {
	return n.initialized
}

// Initialize does the necessary setup for a Namespace
// It does not do the bind mounting and nspinning
func (n *Namespace) Initialize() NamespaceIface {
	n.closed = false
	n.initialized = true
	return n
}

// Creates a new persistent namespace and returns an object
// representing that namespace, without switching to it
func pinNamespaces(nsTypes []NSType, cfg *config.Config) ([]NamespaceIface, error) {
	typeToArg := map[NSType]string{
		IPCNS:  "-i",
		UTSNS:  "-u",
		USERNS: "-U",
		NETNS:  "-n",
	}

	pinDir := filepath.Join(cfg.NamespacesDir, uuid.New().String())

	if err := os.MkdirAll(pinDir, 0755); err != nil {
		return nil, err
	}

	pinnsArgs := []string{"-d", pinDir}
	type namespaceInfo struct {
		path   string
		nsType NSType
	}

	mountedNamespaces := make([]namespaceInfo, 0, len(nsTypes))
	for _, nsType := range nsTypes {
		arg, ok := typeToArg[nsType]
		if !ok {
			return nil, errors.Errorf("Invalid namespace type: %s", nsType)
		}
		pinnsArgs = append(pinnsArgs, arg)
		mountedNamespaces = append(mountedNamespaces, namespaceInfo{
			path:   filepath.Join(pinDir, string(nsType)),
			nsType: nsType,
		})
	}

	pinns := cfg.PinnsPath
	if _, err := exec.Command(pinns, pinnsArgs...).Output(); err != nil {
		// cleanup after ourselves
		failedUmounts := make([]string, 0)
		for _, info := range mountedNamespaces {
			if unmountErr := unix.Unmount(info.path, unix.MNT_DETACH); unmountErr != nil {
				failedUmounts = append(failedUmounts, info.path)
			}
		}
		if len(failedUmounts) != 0 {
			return nil, fmt.Errorf("failed to cleanup %v after pinns failure %v", failedUmounts, err)
		}
		return nil, fmt.Errorf("failed to pin namespaces %v: %v", nsTypes, err)
	}

	returnedNamespaces := make([]NamespaceIface, 0)
	for _, info := range mountedNamespaces {
		ret, err := nspkg.GetNS(info.path)
		if err != nil {
			return nil, err
		}

		returnedNamespaces = append(returnedNamespaces, &Namespace{
			ns:     ret.(NS),
			nsType: info.nsType,
			nsPath: info.path,
		})
	}
	return returnedNamespaces, nil
}

// getNamespace takes a path, checks if it is a namespace, and if so
// returns a Namespace
func getNamespace(nsPath string) (*Namespace, error) {
	if err := nspkg.IsNSorErr(nsPath); err != nil {
		return nil, ErrClosedNS
	}

	ns, err := nspkg.GetNS(nsPath)
	if err != nil {
		return nil, err
	}

	return &Namespace{ns: ns, closed: false, nsPath: nsPath}, nil
}

// Path returns the path of the namespace handle
func (n *Namespace) Path() string {
	if n == nil || n.ns == nil {
		return ""
	}
	return n.nsPath
}

// Type returns which namespace this structure represents
func (n *Namespace) Type() NSType {
	return n.nsType
}

// Close closes this namespace
func (n *Namespace) Close() error {
	if n == nil || n.ns == nil {
		return nil
	}
	return n.ns.Close()
}

// Remove ensures this namespace handle is closed and removed
func (n *Namespace) Remove() error {
	n.Lock()
	defer n.Unlock()

	if n.closed {
		// nsRemove() can be called multiple
		// times without returning an error.
		return nil
	}

	if err := n.Close(); err != nil {
		return err
	}

	n.closed = true

	// we got namespaces in the form of
	// /var/run/$NSTYPEns/$NSTYPE-d08effa-06eb-a963-f51a-e2b0eceffc5d
	// but /var/run on most system is symlinked to /run so we first resolve
	// the symlink and then try and see if it's mounted
	fp, err := symlink.FollowSymlinkInScope(n.Path(), "/")
	if err != nil {
		return err
	}
	if mounted, err := mount.Mounted(fp); err == nil && mounted {
		if err := unix.Unmount(fp, unix.MNT_DETACH); err != nil {
			return err
		}
	}

	if n.Path() != "" {
		if err := os.RemoveAll(n.Path()); err != nil {
			return err
		}
	}

	return nil
}
