package container

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/containers/storage/pkg/mount"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	securejoin "github.com/cyphar/filepath-securejoin"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/selinux/go-selinux"
	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

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

func (c *container) AddOCIBindMounts(ctx context.Context, mountLabel, bindMountPrefix string, absentMountSourcesToReject []string, maybeRelabel, skipRelabel, cgroup2RW, idMapSupport bool, storageRoot string) ([]oci.ContainerVolume, []rspec.Mount, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	volumes := []oci.ContainerVolume{}
	ociMounts := []rspec.Mount{}
	containerConfig := c.Config()
	specgen := c.Spec()
	mounts := containerConfig.Mounts

	// Sort mounts in number of parts. This ensures that high level mounts don't
	// shadow other mounts.
	sort.Sort(criOrderedMounts(mounts))

	// Copy all mounts from default mounts, except for
	// - mounts overridden by supplied mount;
	// - all mounts under /dev if a supplied /dev is present.
	mountSet := make(map[string]struct{})
	for _, m := range mounts {
		mountSet[filepath.Clean(m.ContainerPath)] = struct{}{}
	}
	defaultMounts := specgen.Mounts()
	specgen.ClearMounts()
	for _, m := range defaultMounts {
		dst := filepath.Clean(m.Destination)
		if _, ok := mountSet[dst]; ok {
			// filter out mount overridden by a supplied mount
			continue
		}
		if _, mountDev := mountSet["/dev"]; mountDev && strings.HasPrefix(dst, "/dev/") {
			// filter out everything under /dev if /dev is a supplied mount
			continue
		}
		if _, mountSys := mountSet["/sys"]; mountSys && strings.HasPrefix(dst, "/sys/") {
			// filter out everything under /sys if /sys is a supplied mount
			continue
		}
		specgen.AddMount(m)
	}

	mountInfos, err := mount.GetMounts()
	if err != nil {
		return nil, nil, err
	}
	for _, m := range mounts {
		dest := m.ContainerPath
		if dest == "" {
			return nil, nil, fmt.Errorf("mount.ContainerPath is empty")
		}
		if m.HostPath == "" {
			return nil, nil, fmt.Errorf("mount.HostPath is empty")
		}
		if m.HostPath == "/" && dest == "/" {
			log.Warnf(ctx, "Configuration specifies mounting host root to the container root.  This is dangerous (especially with privileged containers) and should be avoided.")
		}

		if isSubDirectoryOf(storageRoot, m.HostPath) && m.Propagation == types.MountPropagation_PROPAGATION_PRIVATE {
			log.Infof(ctx, "Mount propogration for the host path %s will be set to HostToContainer as it includes the container storage root", m.HostPath)
			m.Propagation = types.MountPropagation_PROPAGATION_HOST_TO_CONTAINER
		}

		src := filepath.Join(bindMountPrefix, m.HostPath)

		resolvedSrc, err := resolveSymbolicLink(bindMountPrefix, src)
		if err == nil {
			src = resolvedSrc
		} else {
			if !os.IsNotExist(err) {
				return nil, nil, fmt.Errorf("failed to resolve symlink %q: %w", src, err)
			}
			for _, toReject := range absentMountSourcesToReject {
				if filepath.Clean(src) == toReject {
					// special-case /etc/hostname, as we don't want it to be created as a directory
					// This can cause issues with node reboot.
					return nil, nil, fmt.Errorf("cannot mount %s: path does not exist and will cause issues as a directory", toReject)
				}
			}
			if !c.Restore() {
				// Although this would also be really helpful for restoring containers
				// it is problematic as during restore external bind mounts need to be
				// a file if the destination is a file. Unfortunately it is not easy
				// to tell if the destination is a file or a directory. Especially if
				// the destination is a nested bind mount. For now we will just not
				// create the missing bind mount source for restore and return an
				// error to the user.
				if err = os.MkdirAll(src, 0o755); err != nil {
					return nil, nil, fmt.Errorf("failed to mkdir %s: %s", src, err)
				}
			}
		}

		options := []string{"rw"}
		if m.Readonly {
			options = []string{"ro"}
		}
		options = append(options, "rbind")

		// mount propagation
		switch m.Propagation {
		case types.MountPropagation_PROPAGATION_PRIVATE:
			options = append(options, "rprivate")
			// Since default root propagation in runc is rprivate ignore
			// setting the root propagation
		case types.MountPropagation_PROPAGATION_BIDIRECTIONAL:
			if err := ensureShared(src, mountInfos); err != nil {
				return nil, nil, err
			}
			options = append(options, "rshared")
			if err := specgen.SetLinuxRootPropagation("rshared"); err != nil {
				return nil, nil, err
			}
		case types.MountPropagation_PROPAGATION_HOST_TO_CONTAINER:
			if err := ensureSharedOrSlave(src, mountInfos); err != nil {
				return nil, nil, err
			}
			options = append(options, "rslave")
			if specgen.Config.Linux.RootfsPropagation != "rshared" &&
				specgen.Config.Linux.RootfsPropagation != "rslave" {
				if err := specgen.SetLinuxRootPropagation("rslave"); err != nil {
					return nil, nil, err
				}
			}
		default:
			log.Warnf(ctx, "Unknown propagation mode for hostPath %q", m.HostPath)
			options = append(options, "rprivate")
		}

		if m.SelinuxRelabel {
			if skipRelabel {
				log.Debugf(ctx, "Skipping relabel for %s because of super privileged container (type: spc_t)", src)
			} else if err := securityLabel(src, mountLabel, false, maybeRelabel); err != nil {
				return nil, nil, err
			}
		} else {
			log.Debugf(ctx, "Skipping relabel for %s because kubelet did not request it", src)
		}

		volumes = append(volumes, oci.ContainerVolume{
			ContainerPath:  dest,
			HostPath:       src,
			Readonly:       m.Readonly,
			Propagation:    m.Propagation,
			SelinuxRelabel: m.SelinuxRelabel,
		})

		uidMappings := getOCIMappings(m.UidMappings)
		gidMappings := getOCIMappings(m.GidMappings)
		if (uidMappings != nil || gidMappings != nil) && !idMapSupport {
			return nil, nil, fmt.Errorf("idmap mounts specified but OCI runtime does not support them. Perhaps the OCI runtime is too old")
		}
		ociMounts = append(ociMounts, rspec.Mount{
			Source:      src,
			Destination: dest,
			Options:     options,
			UIDMappings: uidMappings,
			GIDMappings: gidMappings,
		})
	}

	if _, mountSys := mountSet["/sys"]; !mountSys {
		m := rspec.Mount{
			Destination: "/sys/fs/cgroup",
			Type:        "cgroup",
			Source:      "cgroup",
			Options:     []string{"nosuid", "noexec", "nodev", "relatime"},
		}

		if cgroup2RW {
			m.Options = append(m.Options, "rw")
		} else {
			m.Options = append(m.Options, "ro")
		}
		specgen.AddMount(m)
	}

	return volumes, ociMounts, nil
}

func getOCIMappings(m []*types.IDMapping) []rspec.LinuxIDMapping {
	if len(m) == 0 {
		return nil
	}
	ids := make([]rspec.LinuxIDMapping, 0, len(m))
	for _, m := range m {
		ids = append(ids, rspec.LinuxIDMapping{
			ContainerID: m.ContainerId,
			HostID:      m.HostId,
			Size:        m.Length,
		})
	}
	return ids
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

// Ensure mount point on which path is mounted, is shared.
func ensureShared(path string, mountInfos []*mount.Info) error {
	sourceMount, optionalOpts, err := getSourceMount(path, mountInfos)
	if err != nil {
		return err
	}

	// Make sure source mount point is shared.
	optsSplit := strings.Split(optionalOpts, " ")
	for _, opt := range optsSplit {
		if strings.HasPrefix(opt, "shared:") {
			return nil
		}
	}

	return fmt.Errorf("path %q is mounted on %q but it is not a shared mount", path, sourceMount)
}

// Ensure mount point on which path is mounted, is either shared or slave.
func ensureSharedOrSlave(path string, mountInfos []*mount.Info) error {
	sourceMount, optionalOpts, err := getSourceMount(path, mountInfos)
	if err != nil {
		return err
	}
	// Make sure source mount point is shared.
	optsSplit := strings.Split(optionalOpts, " ")
	for _, opt := range optsSplit {
		if strings.HasPrefix(opt, "shared:") {
			return nil
		} else if strings.HasPrefix(opt, "master:") {
			return nil
		}
	}
	return fmt.Errorf("path %q is mounted on %q but it is not a shared or slave mount", path, sourceMount)
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

func securityLabel(path, secLabel string, shared, maybeRelabel bool) error {
	if maybeRelabel {
		canonicalSecLabel, err := selinux.CanonicalizeContext(secLabel)
		if err != nil {
			logrus.Errorf("Canonicalize label failed %s: %v", secLabel, err)
		} else {
			currentLabel, err := label.FileLabel(path)
			if err == nil && currentLabel == canonicalSecLabel {
				logrus.Debugf(
					"Skipping relabel for %s, as TrySkipVolumeSELinuxLabel is true and the label of the top level of the volume is already correct",
					path)
				return nil
			}
		}
	}
	if err := label.Relabel(path, secLabel, shared); err != nil && !errors.Is(err, unix.ENOTSUP) {
		return fmt.Errorf("relabel failed %s: %w", path, err)
	}
	return nil
}

// AddMountsIfNotExistsInCRI checks if a mount path is in the list of CRI mounts and adds it if not present.
func (c *container) AddMountsIfNotExistsInCRI(dstInfo interface{}, isCheckRequired bool, typeInfo, sourceInfo string, options []string) {
	mountMap := mountSet(c)
	if !isCheckRequired {
		dstStr, ok := dstInfo.(string)
		if !ok {
			logrus.Warn("Unexpected type for destination:", reflect.TypeOf(dstInfo))
			return
		}
		c.SpecAddMount(rspec.Mount{
			Destination: dstStr,
			Type:        typeInfo,
			Source:      sourceInfo,
			Options:     options,
		})
		return
	}
	switch dstInfo := dstInfo.(type) {
	case string:
		_, exists := mountMap[dstInfo]
		if !exists {
			c.SpecAddMount(rspec.Mount{
				Destination: dstInfo,
				Type:        typeInfo,
				Source:      sourceInfo,
				Options:     options,
			})
		}
	case map[string]string:
		// Assuming you want to check if any of the entries in the map are in mountMap
		for target, mode := range dstInfo {
			if _, exists := mountMap[target]; !exists {
				c.SpecAddMount(rspec.Mount{
					Destination: target,
					Type:        typeInfo,
					Source:      sourceInfo,
					Options:     append(options, mode),
				})
			}
		}
	default:
		logrus.Warn("Unexpected type:", reflect.TypeOf(dstInfo))
		return
	}
}
