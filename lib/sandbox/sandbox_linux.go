// +build linux

package sandbox

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/docker/docker/pkg/mount"
	"github.com/docker/docker/pkg/symlink"
	"golang.org/x/sys/unix"
)

func isNSorErr(nspath string) error {
	return ns.IsNSorErr(nspath)
}

func newNetNs() (*NetNs, error) {
	netNS, err := ns.NewNS()
	if err != nil {
		return nil, err
	}

	return &NetNs{netNS: netNS, closed: false}, nil
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
	netNS    ns.NetNS
	symlink  *os.File
	closed   bool
	restored bool
}

func (netns *NetNs) symlinkCreate(name string) error {
	b := make([]byte, 4)
	_, randErr := rand.Reader.Read(b)
	if randErr != nil {
		return randErr
	}

	nsName := fmt.Sprintf("%s-%x", name, b)
	symlinkPath := filepath.Join(NsRunDir, nsName)

	if err := os.Symlink(netns.netNS.Path(), symlinkPath); err != nil {
		return err
	}

	fd, err := os.Open(symlinkPath)
	if err != nil {
		if removeErr := os.RemoveAll(symlinkPath); removeErr != nil {
			return removeErr
		}

		return err
	}

	netns.symlink = fd

	return nil
}

// Path returns the path of the network namespace handle
func (netns *NetNs) Path() string {
	if netns == nil || netns.netNS == nil {
		return ""
	}
	return netns.netNS.Path()
}

// Close closes this network namespace
func (netns *NetNs) Close() error {
	if netns == nil || netns.netNS == nil {
		return nil
	}
	return netns.netNS.Close()
}

// Remove ensures this network namespace handle is closed and removed
func (netns *NetNs) Remove() error {
	netns.Lock()
	defer netns.Unlock()

	if netns.closed {
		// netNsRemove() can be called multiple
		// times without returning an error.
		return nil
	}

	if err := netns.symlinkRemove(); err != nil {
		return err
	}

	if err := netns.Close(); err != nil {
		return err
	}

	netns.closed = true

	if netns.restored {
		// we got namespaces in the form of
		// /var/run/netns/cni-0d08effa-06eb-a963-f51a-e2b0eceffc5d
		// but /var/run on most system is symlinked to /run so we first resolve
		// the symlink and then try and see if it's mounted
		fp, err := symlink.FollowSymlinkInScope(netns.Path(), "/")
		if err != nil {
			return err
		}
		if mounted, err := mount.Mounted(fp); err == nil && mounted {
			if err := unix.Unmount(fp, unix.MNT_DETACH); err != nil {
				return err
			}
		}

		if netns.Path() != "" {
			if err := os.RemoveAll(netns.Path()); err != nil {
				return err
			}
		}
	}

	return nil
}

func (netns *NetNs) symlinkRemove() error {
	if err := netns.symlink.Close(); err != nil {
		return fmt.Errorf("failed to close net ns symlink: %v", err)
	}

	if err := os.RemoveAll(netns.symlink.Name()); err != nil {
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
