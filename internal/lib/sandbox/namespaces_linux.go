// +build linux

package sandbox

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	nspkg "github.com/containernetworking/plugins/pkg/ns"
	"github.com/docker/docker/pkg/mount"
	"github.com/docker/docker/pkg/symlink"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

const pinnsSearchPath string = [
	"/usr/local/libexec/crio/pinns"
	"/usr/libexec/crio/pinns"
	"pinns"
]

func findPinnsPath() string {
	for _, path := range pinnsPath {
		if _, err := exec.LookPath(path); err == nil {
			return path
		}
	}
	return ""
}


// Namespace handles data pertaining to a namespace
type Namespace struct {
	sync.Mutex
	ns          NS
	symlink     *os.File
	closed      bool
	restored    bool
	initialized bool
	nsType      string
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
func (n *Namespace) Initialize(nsType string) NamespaceIface {
	n.nsType = nsType
	n.closed = false
	n.initialized = true
	return n
}

// Creates a new persistent namespace and returns an object
// representing that namespace, without switching to it
func createNewNamespaces(nsTypes []string) ([]*Namespace, error) {
	typeToArg := map[string]string {
		IPCNS: "-i",
		UTSNS: "-u",
		NETNS: "-n",
	}

	// create the namespace dir
	b := make([]byte, 16)
	_, err := rand.Reader.Read(b)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random pinDir name: %v", err)
	}

	const runDir = "/var/run"
	pinDir := fmt.Sprintf("%s/%x-%x-%x-%x-%x", runDir, b[0:4], b[4:6], b[6:8], b[8:10], b[10:])

	err = os.MkdirAll(pinDir, 0755)
	if err != nil {
		return nil, err
	}

	pinnsArgs := []string{"-d", pinDir}
	type namespaceInfo struct{
		path string
		nsType string
	}

	mountedNamespaces := make([]namespaceInfo, 0, len(nsTypes))
	for _, nsType := range nsTypes {
		arg, ok := typeToArg[nsType]
		if !ok {
			return nil, errors.Errorf("Invalid namespace type: %s", nsType)
		}
		pinnsArgs = append(pinnsArgs, arg)
		mountedNamespaces = append(mountedNamespaces, namespaceInfo{
			path: filepath.Join(pinDir, nsType),
			nsType: nsType,
		})
	}

	pinnsPath := findPinnsPath()
	if pinnsPath == "" {
		return nil, errors.Error("Can't find pinns to pin namespaces")
	}
	if _, err := exec.Command(pinnsPath, pinnsArgs...).Output(); err != nil {
		// cleanup after ourselves
		for _, info := range mountedNamespaces {
			failedUmounts := make([]string, 0)
			if unmountErr := unix.Unmount(info.path, unix.MNT_DETACH); unmountErr != nil {
				failedUmounts = append(failedUmounts, info.path)
			}
			return nil, errors.Wrapf(err, "unable to unmount %v", failedUmounts)
		}
		return nil, fmt.Errorf("failed to pin namespaces %v: %v", nsTypes, err)
	}

	returnedNamespaces := make([]*Namespace, 0)
	for _, info := range mountedNamespaces {
		ret, err := nspkg.GetNS(info.path)
		if err != nil {
			return nil, err
		}

		returnedNamespaces = append(returnedNamespaces, &Namespace{
			ns: ret.(NS),
			nsType: info.nsType,
		})
	}
	return returnedNamespaces, nil
}

func getNamespace(nsPath string) (*Namespace, error) {
	ns, err := nspkg.GetNS(nsPath)
	if err != nil {
		return nil, err
	}

	return &Namespace{ns: ns, closed: false, restored: true}, nil
}

// SymlinkCreate creates the necessary symlinks for the Namespace
func (n *Namespace) SymlinkCreate(name string) error {
	if n.ns == nil {
		return errors.New("no ns set up")
	}
	b := make([]byte, 4)
	_, randErr := rand.Reader.Read(b)
	if randErr != nil {
		return randErr
	}

	nsRunDir := getRunDirGivenType(n.nsType)

	nsName := fmt.Sprintf("%s-%x", name, b)
	symlinkPath := filepath.Join(nsRunDir, nsName)

	if err := os.Symlink(n.Path(), symlinkPath); err != nil {
		return err
	}

	fd, err := os.Open(symlinkPath)
	if err != nil {
		if removeErr := os.RemoveAll(symlinkPath); removeErr != nil {
			return removeErr
		}

		return err
	}

	n.symlink = fd

	return nil
}

func getRunDirGivenType(nsType string) string {
	// runDir is the default directory in which running namespaces
	// are stored
	const runDir = "/var/run"
	return fmt.Sprintf("%s/%sns", runDir, nsType)
}

// Path returns the path of the namespace handle
func (n *Namespace) Path() string {
	if n == nil || n.ns == nil {
		return ""
	}
	return n.ns.Path()
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

	if err := n.symlinkRemove(); err != nil {
		return err
	}

	if err := n.Close(); err != nil {
		return err
	}

	n.closed = true

	if n.restored {
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
	}

	return nil
}

func (n *Namespace) symlinkRemove() error {
	if err := n.symlink.Close(); err != nil {
		return fmt.Errorf("failed to close ns symlink: %v", err)
	}

	if err := os.RemoveAll(n.symlink.Name()); err != nil {
		return fmt.Errorf("failed to remove ns symlink: %v", err)
	}

	return nil
}
