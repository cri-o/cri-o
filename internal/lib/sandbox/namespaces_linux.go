// +build linux

package sandbox

import (
	"crypto/rand"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sync"

	nspkg "github.com/containernetworking/plugins/pkg/ns"
	"github.com/docker/docker/pkg/mount"
	"github.com/docker/docker/pkg/symlink"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

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
func (n *Namespace) Initialize(nsType string) (NamespaceIface, error) {
	n.ns = ns
	n.nsType = nsType
	n.closed = false
	n.initialized = true
	return n, nil
}

// Creates a new persistent namespace and returns an object
// representing that namespace, without switching to it
func createNewNamespaces(nsTypes []string) ([]NS, error) {
	type namespaceInfo struct{
		flag int
		path string
		set bool
	}

	typeToFlag := map[string]*namespaceInfo{
		NETNS: &namespaceInfo{
			flag: unix.CLONE_NEWNET,
			set: false,
		},
		IPCNS: &namespaceInfo{
			flag: unix.CLONE_NEWIPC,
			set: false,
		},
		UTSNS: &namespaceInfo{
			flag: unix.CLONE_NEWUTS,
			set: false,
		},
	}

	for nsType := range nsTypes {
		info, ok := typeToFlag[nsType]
		if !ok {
			return nil, fmt.Errorf("invalid namespace type: %s", nsType)
		}

		b := make([]byte, 16)
		_, err := rand.Reader.Read(b)
		if err != nil {
			return nil, fmt.Errorf("failed to generate random %sns name: %v", nsType, err)
		}

		nsRunDir := getRunDirGivenType(nsType)

		err = os.MkdirAll(nsRunDir, 0755)
		if err != nil {
			return nil, err
		}

		// create an empty file at the mount point
		nsName := fmt.Sprintf("%s-%x-%x-%x-%x-%x", nsType, b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
		nsPath := path.Join(nsRunDir, nsName)
		mountPointFd, err := os.Create(nsPath)
		if err != nil {
			return nil, err
		}
		mountPointFd.Close()

		// Ensure the mount point is cleaned up on errors; if the namespace
		// was successfully mounted this will have no effect because the file
		// is in-use
		defer os.RemoveAll(nsPath)
		info.path = nsPath
		info.set = true
	}
	var wg sync.WaitGroup
	wg.Add(1)

	mountedNamespaces := make([]string, 0)
	// do namespace work in a dedicated goroutine, so that we can safely
	// Lock/Unlock OSThread without upsetting the lock/unlock state of
	// the caller of this function
	go (func() {
		defer wg.Done()
		runtime.LockOSThread()

		originalNamespaces := make([]nspkg.NetNS, 0)
		flags := 0

		for nsType, info := range typeToFlag {
			var origNS nspkg.NetNS
			origNS, err = nspkg.GetNS(getCurrentThreadNSPath(nsType))
			if err != nil {
				return
			}
			defer origNS.Close()
			originalNamespaces = append(originalNamespaces, origNS)
			flags |= info.flag
		}

		// create a new ns on the current thread
		// unshare all at once, for efficiency
		err = unix.Unshare(flags)
		if err != nil {
			return
		}

		defer func() {
			for _, origNS := range originalNamespaces {
				if err := origNS.Set(); err != nil {
					logrus.Warnf("unable to set %s namespace: %v", nsType, err)
				}
			}
		}()

		for nsType, info := range typeToFlag {
			// bind mount the new ns from the current thread onto the mount point
			err = unix.Mount(getCurrentThreadNSPath(nsType), info.path, "none", unix.MS_BIND, "")
			if err != nil {
				return
			}
			mountedNamespaces = append(mountedNamespaces(info.path))

			_, err = os.Open(info.path)
			if err != nil {
				return
			}
		}
	})()
	wg.Wait()

	if err != nil {
		for _, nsPath := range mountedNamespaces {
			failedUmounts := make([]string, 0)
			if unmountErr := unix.Unmount(nsPath, unix.MNT_DETACH); err != nil {
				failedUmounts = append(failedUmounts, nsPath)
			}
			return nil, errors.Wrapf(err, "unable to unmount %v", failedUmounts)
		}
		return nil, fmt.Errorf("failed to create %s namespace: %v", nsType, err)
	}

	returnedNamespaces := make([]NamespaceIface, 0)
	for _, info := range mountedNamespaces {
		ret, err := nspkg.GetNS(nsPath)
		if err != nil {
			return nil, err
		}
		returnedNamespaces = append(returnedNamespaces, ret.(NS))
	}
	return returnedNamespaces, nil
}

func getCurrentThreadNSPath(nsType string) string {
	// /proc/self/ns/%s returns the namespace of the main thread, not
	// of whatever thread this goroutine is running on.  Make sure we
	// use the thread's namespace since the thread is switching around
	return fmt.Sprintf("/proc/%d/task/%d/ns/%s", os.Getpid(), unix.Gettid(), nsType)
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
