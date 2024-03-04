package container

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/containers/storage/pkg/mount"
	securejoin "github.com/cyphar/filepath-securejoin"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type mountInfo struct {
	criMounts map[string]*rspec.Mount
	criDevSet bool
	criSysSet bool
	mounts    map[string]*rspec.Mount
}

func newMountInfo() *mountInfo {
	return &mountInfo{
		criMounts: make(map[string]*rspec.Mount),
		mounts:    make(map[string]*rspec.Mount),
	}
}

func clearMountInfo(c *container) {
	c.mountInfo.criMounts = nil
	c.mountInfo.mounts = nil
	c.mountInfo = nil
}

type orderedMounts []*rspec.Mount

// Len returns the number of mounts. Used in sorting.
func (m orderedMounts) Len() int {
	return len(m)
}

// Less returns true if the number of parts (a/b/c would be 3 parts) in the
// mount indexed by parameter 1 is less than that of the mount indexed by
// parameter 2. Used in sorting.
func (m orderedMounts) Less(i, j int) bool {
	return m.parts(i) < m.parts(j)
}

// Swap swaps two items in an array of mounts. Used in sorting
func (m orderedMounts) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

// parts returns the number of parts in the destination of a mount. Used in sorting.
func (m orderedMounts) parts(i int) int {
	return strings.Count(filepath.Clean(m[i].Destination), string(os.PathSeparator))
}

// mounts defines how to sort runtime.Mount.
// This is the same with the Docker implementation:
//
//	https://github.com/moby/moby/blob/17.05.x/daemon/volumes.go#L26
type criOrderedMounts []*types.Mount

// Len returns the number of mounts. Used in sorting.
func (m criOrderedMounts) Len() int {
	return len(m)
}

// Less returns true if the number of parts (a/b/c would be 3 parts) in the
// mount indexed by parameter 1 is less than that of the mount indexed by
// parameter 2. Used in sorting.
func (m criOrderedMounts) Less(i, j int) bool {
	return m.parts(i) < m.parts(j)
}

// Swap swaps two items in an array of mounts. Used in sorting
func (m criOrderedMounts) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

// parts returns the number of parts in the destination of a mount. Used in sorting.
func (m criOrderedMounts) parts(i int) int {
	return strings.Count(filepath.Clean(m[i].ContainerPath), string(os.PathSeparator))
}

func getSourceMount(source string, mountinfos []*mount.Info) (path, optional string, _ error) {
	var res *mount.Info

	for _, mi := range mountinfos {
		// check if mi can be a parent of source
		if strings.HasPrefix(source, mi.Mountpoint) {
			// look for a longest one
			if res == nil || len(mi.Mountpoint) > len(res.Mountpoint) {
				res = mi
			}
		}
	}
	if res == nil {
		return "", "", fmt.Errorf("could not find source mount of %s", source)
	}

	return res.Mountpoint, res.Optional, nil
}

// Ensure mount point on which path is mounted, is shared.
func ensureShared(path string, mountInfos []*mount.Info) error {
	return checkPropagationType(path, mountInfos, "shared")
}

// Ensure mount point on which path is mounted, is either shared or slave.
func ensureSharedOrSlave(path string, mountInfos []*mount.Info) error {
	return checkPropagationType(path, mountInfos, "shared/slave")
}

func checkPropagationType(path string, mountInfos []*mount.Info, propagation string) error {
	sourceMount, optionalOpts, err := getSourceMount(path, mountInfos)
	if err != nil {
		return err
	}
	optsSplit := strings.Split(optionalOpts, " ")
	for _, opt := range optsSplit {
		if strings.HasPrefix(opt, "shared:") && strings.Contains(propagation, "shared") {
			// Make sure source mount point is shared.
			return nil
		} else if strings.HasPrefix(opt, "master:") && strings.Contains(propagation, "slave") {
			// Make sure source mount point is slave.
			return nil
		}
	}
	return fmt.Errorf("path %q is mounted on %q but it is not a %q mount", path, sourceMount, propagation)
}

func clearReadOnly(m *rspec.Mount) {
	var opt []string
	for _, o := range m.Options {
		if o == "rw" {
			return
		} else if o != "ro" {
			opt = append(opt, o)
		}
	}
	m.Options = opt
	m.Options = append(m.Options, "rw")
}

// isSubDirectoryOf checks if the base path contains the target path.
// It assumes that paths are Unix-style with forward slashes ("/").
// It ensures that both paths end with a "/" before comparing, so that "/var/lib" will not incorrectly match "/var/libs".

// The function returns true if the base path starts with the target path, providing a way to check if one directory is a subdirectory of another.

// Examples:

// isSubDirectoryOf("/var/lib/containers/storage", "/") returns true
// isSubDirectoryOf("/var/lib/containers/storage", "/var/lib") returns true
// isSubDirectoryOf("/var/lib/containers/storage", "/var/lib/containers") returns true
// isSubDirectoryOf("/var/lib/containers/storage", "/var/lib/containers/storage") returns true
// isSubDirectoryOf("/var/lib/containers/storage", "/var/lib/containers/storage/extra") returns false
// isSubDirectoryOf("/var/lib/containers/storage", "/va") returns false
// isSubDirectoryOf("/var/lib/containers/storage", "/var/tmp/containers") returns false
func isSubDirectoryOf(base, target string) bool {
	if !strings.HasSuffix(target, "/") {
		target += "/"
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return strings.HasPrefix(base, target)
}

// resolveSymbolicLink resolves a possible symlink path. If the path is a symlink, returns resolved
// path; if not, returns the original path.
// note: strictly SecureJoin is not sufficient, as it does not error when a part of the path doesn't exist
// but simply moves on. If the last part of the path doesn't exist, it may need to be created.
func resolveSymbolicLink(scope, path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != os.ModeSymlink {
		return path, nil
	}
	if scope == "" {
		scope = "/"
	}
	return securejoin.SecureJoin(scope, path)
}

func (c *container) addCriMount(mount *rspec.Mount) {
	if mount != nil {
		dst := filepath.Clean(mount.Destination)
		if dst == "/dev" {
			c.mountInfo.criDevSet = true
		}
		if dst == "/sys" {
			c.mountInfo.criSysSet = true
		}
		c.mountInfo.criMounts[dst] = mount
	}
}

func (c *container) isInCRIMounts(dst string) bool {
	dst = filepath.Clean(dst)
	if _, ok := c.mountInfo.criMounts[dst]; ok ||
		(strings.HasPrefix(dst, "/dev/") && c.mountInfo.criDevSet) ||
		(strings.HasPrefix(dst, "/sys/") && c.mountInfo.criSysSet) {
		return true
	}
	return false
}

func (c *container) isInMounts(dst string) bool {
	_, ok := c.mountInfo.mounts[dst]
	return ok
}

func (c *container) addMount(mount *rspec.Mount) {
	if mount != nil {
		c.mountInfo.mounts[filepath.Clean(mount.Destination)] = mount
	}
}

// specAddMounts add the mounts to the OCI Spec
func specAddMounts(c *container) {
	allmounts := make([]*rspec.Mount, 0, len(c.mountInfo.mounts))
	for k := range c.mountInfo.mounts {
		allmounts = append(allmounts, c.mountInfo.mounts[k])
	}

	// Sort & Add mounts to specgen
	sort.Sort(orderedMounts(allmounts))
	for _, m := range allmounts {
		c.spec.AddMount(*m)
	}
}
