package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/intel/goresctrl/pkg/blockio"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"go.podman.io/storage/pkg/idtools"
	"go.podman.io/storage/pkg/mount"
	"golang.org/x/sys/unix"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	crierrors "k8s.io/cri-api/pkg/errors"

	"github.com/cri-o/cri-o/internal/config/device"
	"github.com/cri-o/cri-o/internal/config/node"
	ctrfactory "github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/ociartifact"
	"github.com/cri-o/cri-o/internal/storage"
	v2 "github.com/cri-o/cri-o/pkg/annotations/v2"
)

const (
	cgroupSysFsPath        = "/sys/fs/cgroup"
	cgroupSysFsSystemdPath = "/sys/fs/cgroup/systemd"
)

// createContainerPlatform performs platform dependent intermediate steps before calling the container's oci.Runtime().CreateContainer().
func (s *Server) createContainerPlatform(ctx context.Context, container *oci.Container, cgroupParent string, idMappings *idtools.IDMappings) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	if idMappings != nil && !container.Spoofed() {
		rootPair := idMappings.RootPair()
		for _, path := range []string{container.BundlePath(), container.MountPoint()} {
			if err := makeAccessible(path, rootPair.UID, rootPair.GID); err != nil {
				return fmt.Errorf("cannot make %s accessible to %d:%d: %w", path, rootPair.UID, rootPair.GID, err)
			}
		}
	}

	return s.ContainerServer.Runtime().CreateContainer(ctx, container, cgroupParent, false)
}

// finalizeUserMapping changes the UID, GID and additional GIDs to reflect the new value in the user namespace.
func (s *Server) finalizeUserMapping(sb *sandbox.Sandbox, specgen *generate.Generator, mappings *idtools.IDMappings) {
	if mappings == nil {
		return
	}

	// if the namespace was configured because of a static configuration, do not attempt any mapping
	if s.defaultIDMappings != nil && !s.defaultIDMappings.Empty() {
		return
	}

	if usernsMode, _ := v2.GetAnnotationValue(sb.Annotations(), v2.UsernsMode); usernsMode == "" {
		return
	}

	specgen.Config.Process.User.UID = toContainer(specgen.Config.Process.User.UID, mappings.UIDs())
	gids := mappings.GIDs()
	specgen.Config.Process.User.GID = toContainer(specgen.Config.Process.User.GID, gids)

	for i := range specgen.Config.Process.User.AdditionalGids {
		gid := toContainer(specgen.Config.Process.User.AdditionalGids[i], gids)
		specgen.Config.Process.User.AdditionalGids[i] = gid
	}
}

// this function takes a container config and makes sure its SecurityContext
// is not nil. If it is, it makes sure to set default values for every field.
func setContainerConfigSecurityContext(containerConfig *types.ContainerConfig) *types.LinuxContainerSecurityContext {
	if containerConfig.GetLinux() == nil {
		containerConfig.Linux = &types.LinuxContainerConfig{}
	}

	if containerConfig.GetLinux().GetSecurityContext() == nil {
		containerConfig.Linux.SecurityContext = newLinuxContainerSecurityContext()
	}

	if containerConfig.GetLinux().GetSecurityContext().GetNamespaceOptions() == nil {
		containerConfig.Linux.SecurityContext.NamespaceOptions = &types.NamespaceOption{}
	}

	if containerConfig.GetLinux().GetSecurityContext().GetSelinuxOptions() == nil {
		containerConfig.Linux.SecurityContext.SelinuxOptions = &types.SELinuxOption{}
	}

	return containerConfig.GetLinux().GetSecurityContext()
}

func disableFipsForContainer(ctr ctrfactory.Container, containerDir string) error {
	// Create a unique filename for the FIPS setting file.
	fileName := filepath.Join(containerDir, "sysctl-fips")
	content := []byte("0\n")

	// Write the value '0' to disable FIPS directly to the file.
	if err := os.WriteFile(fileName, content, 0o444); err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}

	ctr.SpecAddMount(rspec.Mount{
		Destination: "/proc/sys/crypto/fips_enabled",
		Source:      fileName,
		Type:        "bind",
		Options:     []string{"noexec", "nosuid", "nodev", "ro", "bind"},
	})

	return nil
}

func setOCIBindMountsPrivileged(g *generate.Generator) {
	spec := g.Config
	// clear readonly for /sys and cgroup
	for i := range spec.Mounts {
		clearReadOnly(&spec.Mounts[i])
	}

	spec.Linux.ReadonlyPaths = nil
	spec.Linux.MaskedPaths = nil
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

func (s *Server) addOCIBindMounts(ctx context.Context, ctr ctrfactory.Container, ctrInfo *storage.ContainerInfo, maybeRelabel, skipRelabel, cgroup2RW, idMapSupport, rroSupport bool) ([]oci.ContainerVolume, []rspec.Mount, []*safeMountInfo, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	volumes := []oci.ContainerVolume{}
	ociMounts := []rspec.Mount{}
	containerConfig := ctr.Config()
	specgen := ctr.Spec()
	mounts := containerConfig.GetMounts()
	namespace := ctr.SandboxConfig().GetMetadata().GetNamespace()

	// Sort mounts in number of parts. This ensures that high level mounts don't
	// shadow other mounts.
	sort.Sort(criOrderedMounts(mounts))

	// Copy all mounts from default mounts, except for
	// - mounts overridden by supplied mount;
	// - all mounts under /dev if a supplied /dev is present.
	mountSet := make(map[string]struct{})
	for _, m := range mounts {
		mountSet[filepath.Clean(m.GetContainerPath())] = struct{}{}
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
		return nil, nil, nil, err
	}

	imageVolumesPath, err := s.ensureImageVolumesPath(ctx, mounts)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ensure image volumes path: %w", err)
	}

	var safeMounts []*safeMountInfo

	for _, m := range mounts {
		dest := m.GetContainerPath()
		if dest == "" {
			return nil, nil, nil, errors.New("mount.ContainerPath is empty")
		}

		if m.GetImage().GetImage() != "" {
			if s.config.OCIArtifactMountSupport {
				// Try mountArtifact first, and fall back to mountImage if it fails with ErrNotFound
				artifactVolumes, err := s.mountArtifact(ctx, specgen, m, ctrInfo.MountLabel, skipRelabel, maybeRelabel)
				if err == nil {
					volumes = append(volumes, artifactVolumes...)

					continue
				}

				// Don't fall back to an image mount if we encounter an error other than ociartifact.ErrNotFound
				if !errors.Is(err, ociartifact.ErrNotFound) {
					return nil, nil, nil, fmt.Errorf("%w: %w", crierrors.ErrImageVolumeMountFailed, err)
				}

				log.Warnf(ctx, "Artifact mount failed, falling back to image mount: %v", err)
			} else {
				log.Debugf(ctx, "Skipping artifact mount because OCI artifact mount support is disabled")
			}

			volume, safeMount, err := s.mountImage(ctx, specgen, imageVolumesPath, m, ctrInfo.RunDir, namespace)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("%w: %w", crierrors.ErrImageVolumeMountFailed, err)
			}

			volumes = append(volumes, *volume)
			safeMounts = append(safeMounts, safeMount)

			continue
		}

		if m.GetHostPath() == "" {
			return nil, nil, nil, errors.New("mount.HostPath is empty")
		}

		if m.GetHostPath() == "/" && dest == "/" {
			log.Warnf(ctx, "Configuration specifies mounting host root to the container root.  This is dangerous (especially with privileged containers) and should be avoided.")
		}

		if isSubDirectoryOf(s.config.Root, m.GetHostPath()) && m.GetPropagation() == types.MountPropagation_PROPAGATION_PRIVATE {
			log.Infof(ctx, "Mount propogration for the host path %s will be set to HostToContainer as it includes the container storage root", m.GetHostPath())
			m.Propagation = types.MountPropagation_PROPAGATION_HOST_TO_CONTAINER
		}

		src := filepath.Join(s.config.BindMountPrefix, m.GetHostPath())

		resolvedSrc, err := resolveSymbolicLink(s.config.BindMountPrefix, src)
		if err == nil {
			src = resolvedSrc
		} else {
			if !os.IsNotExist(err) {
				return nil, nil, nil, fmt.Errorf("failed to resolve symlink %q: %w", src, err)
			}

			for _, toReject := range s.config.AbsentMountSourcesToReject {
				if filepath.Clean(src) == toReject {
					// special-case /etc/hostname, as we don't want it to be created as a directory
					// This can cause issues with node reboot.
					return nil, nil, nil, fmt.Errorf("cannot mount %s: path does not exist and will cause issues as a directory", toReject)
				}
			}

			if !ctr.Restore() {
				// Although this would also be really helpful for restoring containers
				// it is problematic as during restore external bind mounts need to be
				// a file if the destination is a file. Unfortunately it is not easy
				// to tell if the destination is a file or a directory. Especially if
				// the destination is a nested bind mount. For now we will just not
				// create the missing bind mount source for restore and return an
				// error to the user.
				if err = os.MkdirAll(src, 0o755); err != nil {
					return nil, nil, nil, fmt.Errorf("failed to mkdir %s: %w", src, err)
				}
			}
		}

		options := []string{"rbind"}

		// mount propagation
		switch m.GetPropagation() {
		case types.MountPropagation_PROPAGATION_PRIVATE:
			options = append(options, "rprivate")
			// Since default root propagation in runc is rprivate ignore
			// setting the root propagation
		case types.MountPropagation_PROPAGATION_BIDIRECTIONAL:
			if err := ensureShared(src, mountInfos); err != nil {
				return nil, nil, nil, err
			}

			options = append(options, "rshared")

			if err := specgen.SetLinuxRootPropagation("rshared"); err != nil {
				return nil, nil, nil, err
			}
		case types.MountPropagation_PROPAGATION_HOST_TO_CONTAINER:
			if err := ensureSharedOrSlave(src, mountInfos); err != nil {
				return nil, nil, nil, err
			}

			options = append(options, "rslave")

			if specgen.Config.Linux.RootfsPropagation != "rshared" &&
				specgen.Config.Linux.RootfsPropagation != "rslave" {
				if err := specgen.SetLinuxRootPropagation("rslave"); err != nil {
					return nil, nil, nil, err
				}
			}
		default:
			log.Warnf(ctx, "Unknown propagation mode for hostPath %q", m.GetHostPath())

			options = append(options, "rprivate")
		}

		// Recursive Read-only (RRO) support requires the mount to be
		// read-only and the mount propagation set to private.
		switch {
		case m.GetRecursiveReadOnly() && m.GetReadonly():
			if !rroSupport {
				return nil, nil, nil, fmt.Errorf(
					"recursive read-only mount support is not available for hostPath %q",
					m.GetHostPath(),
				)
			}

			if m.GetPropagation() != types.MountPropagation_PROPAGATION_PRIVATE {
				return nil, nil, nil, fmt.Errorf(
					"recursive read-only mount requires private propagation for hostPath %q, got: %s",
					m.GetHostPath(), m.GetPropagation(),
				)
			}

			options = append(options, "rro")
		case m.GetRecursiveReadOnly():
			return nil, nil, nil, fmt.Errorf(
				"recursive read-only mount conflicts with read-write mount for hostPath %q",
				m.GetHostPath(),
			)
		case m.GetReadonly():
			options = append(options, "ro")
		default:
			options = append(options, "rw")
		}

		if m.GetSelinuxRelabel() {
			if skipRelabel {
				log.Debugf(ctx, "Skipping relabel for %s because of super privileged container (type: spc_t)", src)
			} else if err := securityLabel(src, ctrInfo.MountLabel, false, maybeRelabel); err != nil {
				return nil, nil, nil, err
			}
		} else {
			log.Debugf(ctx, "Skipping relabel for %s because kubelet did not request it", src)
		}

		volumes = append(volumes, oci.ContainerVolume{
			ContainerPath:     dest,
			HostPath:          src,
			Readonly:          m.GetReadonly(),
			RecursiveReadOnly: m.GetRecursiveReadOnly(),
			Propagation:       m.GetPropagation(),
			SelinuxRelabel:    m.GetSelinuxRelabel(),
			Image:             m.GetImage(),
		})

		uidMappings := getOCIMappings(m.GetUidMappings())
		gidMappings := getOCIMappings(m.GetGidMappings())

		if (uidMappings != nil || gidMappings != nil) && !idMapSupport {
			return nil, nil, nil, errors.New("idmap mounts specified but OCI runtime does not support them. Perhaps the OCI runtime is too old")
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
			Destination: cgroupSysFsPath,
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

	return volumes, ociMounts, safeMounts, nil
}

// mountArtifact binds artifact blobs to the container filesystem based on the provided mount configuration.
func (s *Server) mountArtifact(ctx context.Context, specgen *generate.Generator, m *types.Mount, mountLabel string, isSPC, maybeRelabel bool) ([]oci.ContainerVolume, error) {
	artifact, err := s.ArtifactStore().Status(ctx, m.GetImage().GetImage())
	if err != nil {
		return nil, fmt.Errorf("failed to get artifact status: %w", err)
	}

	paths, err := s.ArtifactStore().BlobMountPaths(ctx, artifact, s.config.SystemContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get artifact blob mount paths: %w", err)
	}

	options := []string{"bind", "ro"}
	volumes := make([]oci.ContainerVolume, 0, len(paths))
	selinuxRelabel := true

	if !m.GetSelinuxRelabel() {
		log.Debugf(ctx, "Skipping relabel for %s because kubelet did not request it", m.GetImage().GetImage())

		selinuxRelabel = false
	} else if isSPC {
		log.Debugf(ctx, "Skipping relabel for %s because of super privileged container (type: spc_t)", m.GetImage().GetImage())

		selinuxRelabel = false
	}

	paths, err = FilterMountPathsBySubPath(ctx, m.GetImage().GetImage(), m.GetImageSubPath(), paths)
	if err != nil {
		// This error will get reported directly to the end user
		return nil, err
	}

	for _, path := range paths {
		dest, err := securejoin.SecureJoin(m.GetContainerPath(), path.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to join container path %q and artifact blob path %q: %w", m.GetContainerPath(), path.Name, err)
		}

		if selinuxRelabel {
			if err := securityLabel(path.SourcePath, mountLabel, false, maybeRelabel); err != nil {
				return nil, err
			}
		}

		volumes = append(volumes, oci.ContainerVolume{
			ContainerPath:     m.GetContainerPath(),
			HostPath:          path.SourcePath,
			Readonly:          m.GetReadonly(),
			RecursiveReadOnly: m.GetRecursiveReadOnly(),
			Propagation:       m.GetPropagation(),
			SelinuxRelabel:    m.GetSelinuxRelabel(),
			Image:             &types.ImageSpec{Image: artifact.Digest().Encoded()},
		})

		specgen.AddMount(rspec.Mount{
			Type:        "bind",
			Source:      path.SourcePath,
			Destination: dest,
			Options:     options,
			UIDMappings: getOCIMappings(m.GetUidMappings()),
			GIDMappings: getOCIMappings(m.GetGidMappings()),
		})
	}

	return volumes, nil
}

func FilterMountPathsBySubPath(ctx context.Context, artifact, subPath string, paths []ociartifact.BlobMountPath) (filteredPaths []ociartifact.BlobMountPath, err error) {
	if subPath == "" || subPath == "." {
		return paths, nil
	}

	cleanSubPath := filepath.Clean(subPath) + "/"

	if !slices.ContainsFunc(paths, func(val ociartifact.BlobMountPath) bool {
		return strings.HasPrefix(val.Name, cleanSubPath)
	}) {
		return nil, fmt.Errorf("%w: sub path %q does not exist in OCI artifact volume %q", crierrors.ErrImageVolumeMountFailed, subPath, artifact)
	}

	for _, path := range paths {
		if !strings.HasPrefix(path.Name, cleanSubPath) {
			log.Debugf(ctx, "Skipping to mount artifact path %q because it's not a sub path of %q", path.Name, subPath)

			continue
		}

		newPath := strings.TrimPrefix(path.Name, cleanSubPath)
		log.Debugf(ctx, "Modifying artifact mount path from %q to %q because of user specified sub path %q", path.Name, newPath, cleanSubPath)
		filteredPaths = append(filteredPaths, ociartifact.BlobMountPath{Name: newPath, SourcePath: path.SourcePath})
	}

	return filteredPaths, nil
}

// mountImage adds required image mounts to the provided spec generator and returns a corresponding ContainerVolume.
func (s *Server) mountImage(ctx context.Context, specgen *generate.Generator, imageVolumesPath string, m *types.Mount, runDir, namespace string) (*oci.ContainerVolume, *safeMountInfo, error) {
	if m == nil || m.GetImage() == nil || m.GetImage().GetImage() == "" || m.GetContainerPath() == "" {
		return nil, nil, fmt.Errorf("invalid mount specified: %+v", m)
	}

	log.Debugf(ctx, "Image ref to mount under sub path %q: %s", m.GetImageSubPath(), m.GetImage().GetImage())

	status, err := s.storageImageStatus(ctx, &types.ImageSpec{Image: m.GetImage().GetImage()})
	if err != nil {
		return nil, nil, fmt.Errorf("get storage image status: %w", err)
	}

	if status == nil {
		// Should not happen because the kubelet ensures the image.
		// Or the image could be an artifact.
		return nil, nil, fmt.Errorf("image %q does not exist locally", m.GetImage().GetImage())
	}

	imageID := status.ID.IDStringForOutOfProcessConsumptionOnly()

	// Check the signature of the image
	if err := s.verifyImageSignature(ctx, namespace, m.GetImage().GetUserSpecifiedImage(), status); err != nil {
		return nil, nil, err
	}

	log.Debugf(ctx, "Image ID to mount: %v", imageID)

	options := []string{"ro", "noexec", "nosuid", "nodev"}

	mountPoint, err := s.ContainerServer.Store().MountImage(imageID, options, "")
	if err != nil {
		return nil, nil, fmt.Errorf("mount storage: %w", err)
	}

	log.Infof(ctx, "Image mounted to: %s", mountPoint)

	var safeMount *safeMountInfo
	if m.GetImageSubPath() != "" {
		// Kubernetes already verifies that ImageSubPath:
		// - is a relative path
		// - does not contain any '..'
		//
		// We cannot create non-existing paths within the image volume because
		// they're mounted read-only.
		safeMount, err = safeMountSubPath(mountPoint, m.GetImageSubPath(), runDir)
		if err != nil {
			if errors.Is(err, unix.ENOENT) {
				// This is a user-facing error message from a kubelet event, means we should make it as meaningful as possible.
				return nil, nil, fmt.Errorf("sub path %q does not exist in image volume %q", m.GetImageSubPath(), m.GetImage().GetImage())
			}

			return nil, nil, fmt.Errorf("safe mount sub path: %w", err)
		}

		mountPoint = safeMount.mountPoint
		log.Debugf(ctx, "Using final mount path: %s", mountPoint)
	}

	const overlay = "overlay"

	specgen.AddMount(rspec.Mount{
		Type:        overlay,
		Source:      overlay,
		Destination: m.GetContainerPath(),
		Options: []string{
			"lowerdir=" + mountPoint + ":" + imageVolumesPath,
		},
		UIDMappings: getOCIMappings(m.GetUidMappings()),
		GIDMappings: getOCIMappings(m.GetGidMappings()),
	})
	log.Debugf(ctx, "Added overlay mount from %s to %s", mountPoint, imageVolumesPath)

	return &oci.ContainerVolume{
		ContainerPath:     m.GetContainerPath(),
		HostPath:          mountPoint,
		Readonly:          m.GetReadonly(),
		RecursiveReadOnly: m.GetRecursiveReadOnly(),
		Propagation:       m.GetPropagation(),
		SelinuxRelabel:    m.GetSelinuxRelabel(),
		Image:             &types.ImageSpec{Image: imageID},
	}, safeMount, nil
}

func (s *Server) ensureImageVolumesPath(ctx context.Context, mounts []*types.Mount) (string, error) {
	// Check if we need to anything at all
	if !slices.ContainsFunc(mounts, func(m *types.Mount) bool {
		if m.GetImage() != nil && m.GetImage().GetImage() != "" {
			return true
		}

		return false
	}) {
		return "", nil
	}

	imageVolumesPath := filepath.Join(filepath.Dir(s.ContainerServer.Config().ContainerExitsDir), "image-volumes")
	log.Debugf(ctx, "Using image volumes path: %s", imageVolumesPath)

	if err := os.MkdirAll(imageVolumesPath, 0o700); err != nil {
		return "", fmt.Errorf("create image volumes path: %w", err)
	}

	f, err := os.Open(imageVolumesPath)
	if err != nil {
		return "", fmt.Errorf("open image volumes path %s: %w", imageVolumesPath, err)
	}

	_, readErr := f.ReadDir(1)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return "", fmt.Errorf("unable to read dir names of image volumes path %s: %w", imageVolumesPath, readErr)
	}

	if readErr == nil {
		return "", fmt.Errorf("image volumes path %s is not empty", imageVolumesPath)
	}

	return imageVolumesPath, nil
}

func getOCIMappings(m []*types.IDMapping) []rspec.LinuxIDMapping {
	if len(m) == 0 {
		return nil
	}

	ids := make([]rspec.LinuxIDMapping, 0, len(m))
	for _, m := range m {
		ids = append(ids, rspec.LinuxIDMapping{
			ContainerID: m.GetContainerId(),
			HostID:      m.GetHostId(),
			Size:        m.GetLength(),
		})
	}

	return ids
}

// mountExists returns true if dest exists in the list of mounts.
func mountExists(specMounts []rspec.Mount, dest string) bool {
	for _, m := range specMounts {
		if m.Destination == dest {
			return true
		}
	}

	return false
}

// systemd expects to have /run, /run/lock and /tmp on tmpfs
// It also expects to be able to write to /sys/fs/cgroup/systemd and /var/log/journal.
func setupSystemd(mounts []rspec.Mount, g generate.Generator) {
	options := []string{"rw", "rprivate", "noexec", "nosuid", "nodev"}

	for _, dest := range []string{"/run", "/run/lock"} {
		if mountExists(mounts, dest) {
			continue
		}

		tmpfsMnt := rspec.Mount{
			Destination: dest,
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     append(options, "tmpcopyup"),
		}
		g.AddMount(tmpfsMnt)
	}

	for _, dest := range []string{"/tmp", "/var/log/journal"} {
		if mountExists(mounts, dest) {
			continue
		}

		tmpfsMnt := rspec.Mount{
			Destination: dest,
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     append(options, "tmpcopyup"),
		}
		g.AddMount(tmpfsMnt)
	}

	if node.CgroupIsV2() {
		g.RemoveMount(cgroupSysFsPath)

		systemdMnt := rspec.Mount{
			Destination: cgroupSysFsPath,
			Type:        "cgroup",
			Source:      "cgroup",
			Options:     []string{"private", "rw"},
		}
		g.AddMount(systemdMnt)
	} else {
		// If the /sys/fs/cgroup is bind mounted from the host,
		// then systemd-mode cgroup should be disabled
		// https://bugzilla.redhat.com/show_bug.cgi?id=2064741
		if !hasCgroupMount(g.Mounts()) {
			systemdMnt := rspec.Mount{
				Destination: cgroupSysFsSystemdPath,
				Type:        "bind",
				Source:      cgroupSysFsSystemdPath,
				Options:     []string{"bind", "nodev", "noexec", "nosuid"},
			}
			g.AddMount(systemdMnt)
		}

		g.AddLinuxMaskedPaths(filepath.Join(cgroupSysFsSystemdPath, "release_agent"))
	}

	g.AddProcessEnv("container", "crio")
}

func hasCgroupMount(mounts []rspec.Mount) bool {
	for _, m := range mounts {
		if (m.Destination == cgroupSysFsPath || m.Destination == "/sys/fs" || m.Destination == "/sys") && isBindMount(m.Options) {
			return true
		}
	}

	return false
}

func isBindMount(mountOptions []string) bool {
	for _, option := range mountOptions {
		if option == "bind" || option == "rbind" {
			return true
		}
	}

	return false
}

func newLinuxContainerSecurityContext() *types.LinuxContainerSecurityContext {
	return &types.LinuxContainerSecurityContext{
		Capabilities:     &types.Capability{},
		NamespaceOptions: &types.NamespaceOption{},
		SelinuxOptions:   &types.SELinuxOption{},
		RunAsUser:        &types.Int64Value{},
		RunAsGroup:       &types.Int64Value{},
		Seccomp:          &types.SecurityProfile{},
		Apparmor:         &types.SecurityProfile{},
	}
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
// isSubDirectoryOf("/var/lib/containers/storage", "/var/tmp/containers") returns false.
func isSubDirectoryOf(base, target string) bool {
	if !strings.HasSuffix(target, "/") {
		target += "/"
	}

	if !strings.HasSuffix(base, "/") {
		base += "/"
	}

	return strings.HasPrefix(base, target)
}

// Returns the spec Generator for the container, with some values set.
func (s *Server) getSpecGen(ctr ctrfactory.Container, containerConfig *types.ContainerConfig) *generate.Generator {
	specgen := ctr.Spec()
	specgen.HostSpecific = true
	specgen.ClearProcessRlimits()

	for _, u := range s.config.Ulimits() {
		specgen.AddProcessRlimits(u.Name, u.Hard, u.Soft)
	}

	specgen.SetRootReadonly(ctr.ReadOnly(s.config.ReadOnly))

	if s.config.ReadOnly {
		// tmpcopyup is a runc extension and is not part of the OCI spec.
		// WORK ON: Use "overlay" mounts as an alternative to tmpfs with tmpcopyup
		// Look at https://github.com/cri-o/cri-o/pull/1434#discussion_r177200245 for more info on this
		options := []string{"rw", "noexec", "nosuid", "nodev", "tmpcopyup"}

		mounts := map[string]string{
			"/run":     "mode=0755",
			"/tmp":     "mode=1777",
			"/var/tmp": "mode=1777",
		}
		for target, mode := range mounts {
			if !isInCRIMounts(target, containerConfig.GetMounts()) {
				ctr.SpecAddMount(rspec.Mount{
					Destination: target,
					Type:        "tmpfs",
					Source:      "tmpfs",
					Options:     append(options, mode),
				})
			}
		}
	}

	return specgen
}

func (s *Server) specSetApparmorProfile(ctx context.Context, specgen *generate.Generator, ctr ctrfactory.Container, securityContext *types.LinuxContainerSecurityContext) error {
	// set this container's apparmor profile if it is set by sandbox
	if s.ContainerServer.Config().AppArmor().IsEnabled() && !ctr.Privileged() {
		profile, err := s.ContainerServer.Config().AppArmor().Apply(securityContext)
		if err != nil {
			return fmt.Errorf("applying apparmor profile to container %s: %w", ctr.ID(), err)
		}

		log.Debugf(ctx, "Applied AppArmor profile %s to container %s", profile, ctr.ID())
		specgen.SetProcessApparmorProfile(profile)
	}

	return nil
}

func (s *Server) specSetBlockioClass(specgen *generate.Generator, containerName string, containerAnnotations, sandboxAnnotations map[string]string) error {
	// Get blockio class
	if s.ContainerServer.Config().BlockIO().Enabled() {
		if blockioClass, err := blockio.ContainerClassFromAnnotations(containerName, containerAnnotations, sandboxAnnotations); blockioClass != "" && err == nil {
			if s.ContainerServer.Config().BlockIO().ReloadRequired() {
				if err := s.ContainerServer.Config().BlockIO().Reload(); err != nil {
					return err
				}
			}

			if linuxBlockIO, err := blockio.OciLinuxBlockIO(blockioClass); err == nil {
				if specgen.Config.Linux.Resources == nil {
					specgen.Config.Linux.Resources = &rspec.LinuxResources{}
				}

				specgen.Config.Linux.Resources.BlockIO = linuxBlockIO
			}
		}
	}

	return nil
}

func (s *Server) specSetDevices(ctr ctrfactory.Container, sb *sandbox.Sandbox) error {
	configuredDevices := s.config.Devices()

	privilegedWithoutHostDevices, err := s.ContainerServer.Runtime().PrivilegedWithoutHostDevices(sb.RuntimeHandler())
	if err != nil {
		return err
	}

	devicesAnnotationValue, _ := v2.GetAnnotationValue(sb.Annotations(), v2.Devices)

	annotationDevices, err := device.DevicesFromAnnotation(devicesAnnotationValue, s.config.AllowedDevices)
	if err != nil {
		return err
	}

	return ctr.SpecAddDevices(configuredDevices, annotationDevices, privilegedWithoutHostDevices, s.config.DeviceOwnershipFromSecurityContext)
}

func addSysfsMounts(ctr ctrfactory.Container, containerConfig *types.ContainerConfig, hostNet bool) {
	// If the sandbox is configured to run in the host network, do not create a new network namespace
	if hostNet {
		if !isInCRIMounts("/sys", containerConfig.GetMounts()) {
			ctr.SpecAddMount(rspec.Mount{
				Destination: "/sys",
				Type:        "sysfs",
				Source:      "sysfs",
				Options:     []string{"nosuid", "noexec", "nodev", "ro"},
			})
			ctr.SpecAddMount(rspec.Mount{
				Destination: cgroupSysFsPath,
				Type:        "cgroup",
				Source:      "cgroup",
				Options:     []string{"nosuid", "noexec", "nodev", "relatime", "ro"},
			})
		}
	}

	if ctr.Privileged() {
		ctr.SpecAddMount(rspec.Mount{
			Destination: "/sys",
			Type:        "sysfs",
			Source:      "sysfs",
			Options:     []string{"nosuid", "noexec", "nodev", "rw", "rslave"},
		})
		ctr.SpecAddMount(rspec.Mount{
			Destination: cgroupSysFsPath,
			Type:        "cgroup",
			Source:      "cgroup",
			Options:     []string{"nosuid", "noexec", "nodev", "rw", "relatime", "rslave"},
		})
	}
}

func addShmMount(ctr ctrfactory.Container, sb *sandbox.Sandbox) {
	ctr.SpecAddMount(rspec.Mount{
		Destination: "/dev/shm",
		Type:        "bind",
		Source:      sb.ShmPath(),
		Options:     []string{"rw", "bind"},
	})
}
