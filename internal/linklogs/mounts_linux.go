package linklogs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.podman.io/storage/pkg/mount"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/util/validation"
)

// Mounts identifies sources backed by the selected log volume, including bind aliases.
type Mounts struct {
	root   string
	device uint64
	infos  map[int]*mount.Info
}

// NewMounts returns nil when log linking has no valid, existing volume to protect.
func NewMounts(podUID, volumeName string, infos []*mount.Info) (*Mounts, error) {
	if podUID == "" || len(validation.IsDNS1123Label(volumeName)) != 0 {
		return nil, nil
	}

	path, err := podEmptyDirPath(podUID, volumeName)
	if err != nil {
		return nil, err
	}

	return newMounts(path, infos)
}

func newMounts(path string, infos []*mount.Info) (*Mounts, error) {
	m := &Mounts{infos: make(map[int]*mount.Info, len(infos))}
	for _, info := range infos {
		m.infos[info.ID] = info
	}

	root, device, err := m.source(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	m.root, m.device = root, device

	return m, nil
}

// Contains reports whether source is the log directory or one of its descendants.
//
// Kubelet exposes subPath mounts through bind aliases. For example, the hostPath
// /var/lib/kubelet/pods/<uid>/volume-subpaths/cache/app/0 can refer to a directory
// under /var/lib/kubelet/pods/<uid>/volumes/kubernetes.io~empty-dir/cache.
// Checking the emptyDir path prefix would miss this alias, and resolving symlinks
// does not resolve bind mounts. Matching the backing filesystem and paths ensures
// these subPath mounts receive the same read-only protection as the log volume.
func (m *Mounts) Contains(source string) (bool, error) {
	if m == nil || !filepath.IsAbs(source) {
		return false, nil
	}

	root, device, err := m.source(source)
	if err != nil {
		return false, err
	}

	if device != m.device {
		return false, nil
	}

	rel, err := filepath.Rel(m.root, root)

	return err == nil && filepath.IsLocal(rel), err
}

// source identifies the backing filesystem and the path within it, so bind
// aliases can be compared regardless of where they appear on the host.
//
// For example, if a filesystem's root is mounted at /var/log, both
// /var/log/pods/pod1/app and a bind alias of it at /mnt/subpath resolve to
// /pods/pod1/app within that filesystem.
func (m *Mounts) source(path string) (root string, device uint64, err error) {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", 0, fmt.Errorf("open mount source %q: %w", path, err)
	}
	defer unix.Close(fd)

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return "", 0, err
	}

	// Compare stat devices rather than mountinfo devices: Btrfs subvolumes
	// can report different values. Unrelated filesystems need no lookup.
	if m.root != "" && stat.Dev != m.device {
		return "", stat.Dev, nil
	}

	resolved, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
	if err != nil {
		return "", 0, err
	}

	data, err := os.ReadFile(fmt.Sprintf("/proc/self/fdinfo/%d", fd))
	if err != nil {
		return "", 0, err
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		value, ok := strings.CutPrefix(line, "mnt_id:")
		if !ok {
			continue
		}

		id, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return "", 0, err
		}

		info, ok := m.infos[id]
		if !ok {
			// The source may have been mounted since the snapshot was taken.
			infos, err := mount.GetMounts()
			if err != nil {
				return "", 0, err
			}

			for _, current := range infos {
				if current.ID == id {
					info = current

					break
				}
			}

			if info == nil {
				return "", 0, fmt.Errorf(
					"mount ID %d for %q is missing from mount table",
					id,
					path,
				)
			}
		}

		rel, err := filepath.Rel(info.Mountpoint, resolved)
		if err != nil || !filepath.IsLocal(rel) {
			return "", 0, fmt.Errorf(
				"mount source %q is outside mount point %q",
				resolved,
				info.Mountpoint,
			)
		}

		return filepath.Join(info.Root, rel), stat.Dev, nil
	}

	return "", 0, fmt.Errorf("mount ID for %q is missing from fdinfo", path)
}
