// +build linux

package sandbox

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/docker/docker/pkg/mount"
	"github.com/docker/docker/pkg/symlink"
	"golang.org/x/sys/unix"
)

// Get returns the NetNs for a given NetNsIface
func (n *NetNs) Get() *NetNs {
	return n
}

// Initialized returns true if the NetNs is already initialized
func (n *NetNs) Initialized() bool {
	return n.initialized
}

// Initialize does the necessary setup for a NetNs
func (n *NetNs) Initialize() (NetNsIface, error) {
	netNS, err := ns.NewNS()
	if err != nil {
		return nil, err
	}
	n.netNS = netNS
	n.closed = false
	n.initialized = true
	return n, nil
}

func getNetNs(path string) (*NetNs, error) {
	netNS, err := ns.GetNS(path)
	if err != nil {
		return nil, err
	}

	return &NetNs{netNS: netNS, closed: false, restored: true}, nil
}

// NetNs handles data pertaining a network namespace
type NetNs struct {
	sync.Mutex
	netNS       ns.NetNS
	symlink     *os.File
	closed      bool
	restored    bool
	initialized bool
}

// SymlinkCreate creates the necessary symlinks for the NetNs
func (n *NetNs) SymlinkCreate(name string) error {
	if n.netNS == nil {
		return errors.New("no netns set up")
	}
	b := make([]byte, 4)
	_, randErr := rand.Reader.Read(b)
	if randErr != nil {
		return randErr
	}

	nsName := fmt.Sprintf("%s-%x", name, b)
	symlinkPath := filepath.Join(NsRunDir, nsName)

	if err := os.Symlink(n.netNS.Path(), symlinkPath); err != nil {
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

// Path returns the path of the network namespace handle
func (n *NetNs) Path() string {
	if n == nil || n.netNS == nil {
		return ""
	}
	return n.netNS.Path()
}

// Close closes this network namespace
func (n *NetNs) Close() error {
	if n == nil || n.netNS == nil {
		return nil
	}
	return n.netNS.Close()
}

// Remove ensures this network namespace handle is closed and removed
func (n *NetNs) Remove() error {
	n.Lock()
	defer n.Unlock()

	if n.closed {
		// netNsRemove() can be called multiple
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
		// /var/run/netns/cni-0d08effa-06eb-a963-f51a-e2b0eceffc5d
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

func (n *NetNs) symlinkRemove() error {
	if err := n.symlink.Close(); err != nil {
		return fmt.Errorf("failed to close net ns symlink: %v", err)
	}

	if err := os.RemoveAll(n.symlink.Name()); err != nil {
		return fmt.Errorf("failed to remove net ns symlink: %v", err)
	}

	return nil
}

func hostNetNsPath() (string, error) {
	netNS, err := ns.GetCurrentNS()
	if err != nil {
		return "", err
	}

	defer netNS.Close()
	return netNS.Path(), nil
}
