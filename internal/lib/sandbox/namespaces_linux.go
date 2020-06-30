// +build linux

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	nspkg "github.com/containernetworking/plugins/pkg/ns"
	"github.com/containers/storage/pkg/mount"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/utils"
	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
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

type namespaceInfo struct {
	path   string
	nsType NSType
}

func newNamespaceInfo(nsDir, nsFile string, nsType NSType) *namespaceInfo {
	return &namespaceInfo{
		path:   filepath.Join(nsDir, fmt.Sprintf("%sns", string(nsType)), nsFile),
		nsType: nsType,
	}
}

func (info *namespaceInfo) toIface() (NamespaceIface, error) {
	ret, err := nspkg.GetNS(info.path)
	if err != nil {
		return nil, err
	}

	return &Namespace{
		ns:     ret.(NS),
		nsType: info.nsType,
		nsPath: info.path,
	}, nil
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

	numNSToPin := len(nsTypes)

	pinnedNamespace := uuid.New().String()
	pinnsArgs := []string{
		"-d", cfg.NamespacesDir,
		"-f", pinnedNamespace,
	}

	mountedNamespaces := make([]*namespaceInfo, 0, numNSToPin)
	for _, nsType := range nsTypes {
		arg, ok := typeToArg[nsType]
		if !ok {
			return nil, errors.Errorf("Invalid namespace type: %s", nsType)
		}
		pinnsArgs = append(pinnsArgs, arg)
		mountedNamespaces = append(mountedNamespaces, newNamespaceInfo(cfg.NamespacesDir, pinnedNamespace, nsType))
	}
	pinns := cfg.PinnsPath

	logrus.Debugf("calling pinns with %v", pinnsArgs)
	output, err := exec.Command(pinns, pinnsArgs...).Output()
	if len(output) != 0 {
		logrus.Debugf("pinns output: %s", string(output))
	}
	if err != nil {
		// cleanup after ourselves
		failedUmounts := make([]string, 0)
		for _, info := range mountedNamespaces {
			if unmountErr := utils.Unmount(info.path, unix.MNT_DETACH); unmountErr != nil {
				failedUmounts = append(failedUmounts, info.path)
			}
		}
		if len(failedUmounts) != 0 {
			return nil, fmt.Errorf("failed to cleanup %v after pinns failure %s %v", failedUmounts, output, err)
		}
		return nil, fmt.Errorf("failed to pin namespaces %v: %s %v", nsTypes, output, err)
	}

	returnedNamespaces := make([]NamespaceIface, 0, numNSToPin)
	for _, info := range mountedNamespaces {
		iface, err := info.toIface()
		if err != nil {
			return nil, err
		}
		returnedNamespaces = append(returnedNamespaces, iface)
	}
	return returnedNamespaces, nil
}

func pinPidNamespace(cfg *config.Config, path string) (iface NamespaceIface, errRet error) {
	// verify the path we were passed is indeed a namespace
	if err := nspkg.IsNSorErr(path); err != nil {
		return nil, err
	}
	pinnedNamespace := uuid.New().String()
	namespaceInfo := newNamespaceInfo(cfg.NamespacesDir, pinnedNamespace, PIDNS)

	// ensure the parent directory is there
	if err := os.MkdirAll(filepath.Join(cfg.NamespacesDir, "pidns"), 0o755); err != nil {
		return nil, err
	}

	// now create an empty file
	f, err := os.Create(namespaceInfo.path)
	if err != nil {
		return nil, err
	}
	f.Close()

	// Ensure the mount point is cleaned up on errors; if the namespace
	// was successfully mounted this will have no effect because the file
	// is in-use
	defer os.RemoveAll(namespaceInfo.path)

	// bind mount the new netns from the pidns entry onto the mount point

	if err := utils.Mount(path, namespaceInfo.path, "none", unix.MS_BIND, ""); err != nil {
		return nil, err
	}
	defer func() {
		if errRet != nil {
			if err := utils.Unmount(namespaceInfo.path, unix.MNT_DETACH); err != nil {
				logrus.Errorf("failed umount after failed to pin pid namespace: %v", err)
			}
		}
	}()

	return namespaceInfo.toIface()
}

// getNamespace takes a path, checks if it is a namespace, and if so
// returns a Namespace
func getNamespace(nsPath string) (*Namespace, error) {
	if err := nspkg.IsNSorErr(nsPath); err != nil {
		return nil, err
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
	fp, err := securejoin.SecureJoin("/", n.Path())
	if err != nil {
		return errors.Wrapf(err, "unable to join '/' with %s path", n.Path())
	}
	mounted, err := mount.Mounted(fp)
	if err != nil {
		return errors.Wrap(err, "unable to check if path is mounted")
	}
	if mounted {
		if err := utils.Unmount(fp, unix.MNT_DETACH); err != nil {
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
