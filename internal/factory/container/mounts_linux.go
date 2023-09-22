package container

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/containers/common/pkg/subscriptions"
	"github.com/containers/podman/v4/pkg/rootless"
	"github.com/containers/podman/v4/pkg/selinux"
	"github.com/containers/storage/pkg/idtools"
	"github.com/containers/storage/pkg/mount"
	"github.com/containers/storage/pkg/stringid"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/resourcestore"
	"github.com/cri-o/cri-o/internal/storage"
	crioann "github.com/cri-o/cri-o/pkg/annotations"
	"github.com/cri-o/cri-o/pkg/config"
	sconfig "github.com/cri-o/cri-o/pkg/config"
	securejoin "github.com/cyphar/filepath-securejoin"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (ctr *container) setupMounts(ctx context.Context, resourceStore *resourcestore.ResourceStore, serverConfig *sconfig.Config, sb *sandbox.Sandbox, containerInfo storage.ContainerInfo, mountPoint string, idMapSupport bool) ([]oci.ContainerVolume, []rspec.Mount, error) {

	options := []string{"rw"}
	if serverConfig.ReadOnly {
		options = []string{"ro"}
	}

	// Setup readonly mounts
	ctr.setupReadOnlyMounts(serverConfig.ReadOnly)

	// add OCI default & bind mounts
	maybeRelabel := false
	if val, present := sb.Annotations()[crioann.TrySkipVolumeSELinuxLabelAnnotation]; present && val == "true" {
		maybeRelabel = true
	}
	cgroup2RW := node.CgroupIsV2() && sb.Annotations()[crioann.Cgroup2RWAnnotation] == "true"
	resourceStore.SetStageForResource(ctx, ctr.Name(), "container volume configuration")
	containerVolumes, err := ctr.addOCIBindMounts(ctx, containerInfo.MountLabel, serverConfig, maybeRelabel, idMapSupport, cgroup2RW)
	if err != nil {
		return nil, nil, err
	}

	// If the sandbox is configured to run in the host network, do not create a new network namespace
	ctr.setupHostNetworkMounts(sb, options)

	// Setup Privileged mounts
	ctr.setupPrivilegedMounts()

	// Setup Shared Memory mounts
	ctr.setupShmMounts(sb)

	// Setup Host Properties like hostname, env, dns
	if err := ctr.setupHostPropMounts(sb, containerInfo.MountLabel, options); err != nil {
		return nil, nil, err
	}

	// Clear readonly flags
	ctr.setOCIBindMountsPrivileged()

	// Add image volumes
	if err := ctr.addImageVolumes(ctx, mountPoint, serverConfig, &containerInfo); err != nil {
		return nil, nil, err
	}

	// Add secrets from the default and override mounts.conf files
	secretMounts := ctr.setupSecretMounts(serverConfig, containerInfo, mountPoint)

	// Setup systemd mounts if process args are configured to run
	// as systemd instance
	if err := ctr.setupSystemdMounts(containerInfo); err != nil {
		return nil, nil, err
	}

	return containerVolumes, secretMounts, nil
}

func (ctr *container) setupHostNetworkMounts(sb *sandbox.Sandbox, options []string) {
	if sb.HostNetwork() {
		ctr.addMount(&rspec.Mount{
			Destination: "/sys",
			Type:        "sysfs",
			Source:      "sysfs",
			Options:     []string{"nosuid", "noexec", "nodev", "ro"},
		})
		ctr.addMount(&rspec.Mount{
			Destination: "/sys/fs/cgroup",
			Type:        "cgroup",
			Source:      "cgroup",
			Options:     []string{"nosuid", "noexec", "nodev", "relatime", "ro"},
		})
		// Only bind mount for host netns
		ctr.addMount(&rspec.Mount{
			Destination: "/etc/hosts",
			Type:        "bind",
			Source:      "/etc/hosts",
			Options:     append(options, "bind"),
		})
	}
}

func (ctr *container) setupPrivilegedMounts() {
	if ctr.Privileged() {
		ctr.addMount(&rspec.Mount{
			Destination: "/sys",
			Type:        "sysfs",
			Source:      "sysfs",
			Options:     []string{"nosuid", "noexec", "nodev", "rw", "rslave"},
		})
		ctr.addMount(&rspec.Mount{
			Destination: "/sys/fs/cgroup",
			Type:        "cgroup",
			Source:      "cgroup",
			Options:     []string{"nosuid", "noexec", "nodev", "rw", "relatime", "rslave"},
		})
	}
}

func (ctr *container) setupReadOnlyMounts(readOnly bool) {
	if readOnly {
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
			ctr.addMount(&rspec.Mount{
				Destination: target,
				Type:        "tmpfs",
				Source:      "tmpfs",
				Options:     append(options, mode),
			})
		}
	}
}

func (ctr *container) setupShmMounts(sb *sandbox.Sandbox) {
	ctr.addMount(&rspec.Mount{
		Destination: "/dev/shm",
		Type:        "bind",
		Source:      sb.ShmPath(),
		Options:     []string{"rw", "bind"},
	})
}

func (ctr *container) addHostPropMounts(mount *rspec.Mount, mountLabel string) error {
	if mount.Source != "" {
		if err := SecurityLabel(mount.Source, mountLabel, false, false); err != nil {
			return err
		}
	}
	ctr.addMount(mount)
	return nil
}

func (ctr *container) setupHostPropMounts(sb *sandbox.Sandbox, mountLabel string, options []string) error {
	hostMounts := []rspec.Mount{
		rspec.Mount{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      sb.ResolvPath(),
			Options:     append(options, []string{"bind", "nodev", "nosuid", "noexec"}...),
		},
		rspec.Mount{
			Destination: "/etc/hostname",
			Type:        "bind",
			Source:      sb.HostnamePath(),
			Options:     append(options, "bind"),
		},
		rspec.Mount{
			Destination: "/run/.containerenv",
			Type:        "bind",
			Source:      sb.ContainerEnvPath(),
			Options:     append(options, "bind"),
		},
	}

	for _, mount := range hostMounts {
		if err := ctr.addHostPropMounts(&mount, mountLabel); err != nil {
			return err
		}
	}
	return nil
}

func (ctr *container) setOCIBindMountsPrivileged() {
	if ctr.Privileged() {
		spec := ctr.Spec().Config
		// clear readonly for /sys and cgroup
		for i := range spec.Mounts {
			clearReadOnly(&spec.Mounts[i])
		}
		spec.Linux.ReadonlyPaths = nil
		spec.Linux.MaskedPaths = nil
	}
}

func (ctr *container) addImageVolumes(ctx context.Context, rootfs string, serverConfig *sconfig.Config, containerInfo *storage.ContainerInfo) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	for dest := range containerInfo.Config.Config.Volumes {
		fp, err := securejoin.SecureJoin(rootfs, dest)
		if err != nil {
			return err
		}
		switch serverConfig.ImageVolumes {
		case config.ImageVolumesMkdir:
			IDs := idtools.IDPair{UID: int(ctr.Spec().Config.Process.User.UID), GID: int(ctr.Spec().Config.Process.User.GID)}
			if err1 := idtools.MkdirAllAndChownNew(fp, 0o755, IDs); err1 != nil {
				return err1
			}
			if containerInfo.MountLabel != "" {
				if err1 := SecurityLabel(fp, containerInfo.MountLabel, true, false); err1 != nil {
					return err1
				}
			}
		case config.ImageVolumesBind:
			volumeDirName := stringid.GenerateNonCryptoID()
			src := filepath.Join(containerInfo.RunDir, "mounts", volumeDirName)
			if err1 := os.MkdirAll(src, 0o755); err1 != nil {
				return err1
			}
			// Label the source with the sandbox selinux mount label
			if containerInfo.MountLabel != "" {
				if err1 := SecurityLabel(src, containerInfo.MountLabel, true, false); err1 != nil {
					return err1
				}
			}

			log.Debugf(ctx, "Adding bind mounted volume: %s to %s", src, dest)
			ctr.addMount(&rspec.Mount{
				Source:      src,
				Destination: dest,
				Type:        "bind",
				Options:     []string{"private", "bind", "rw"},
			})
		case config.ImageVolumesIgnore:
			log.Debugf(ctx, "Ignoring volume %v", dest)
		default:
			log.Errorf(ctx, "Unrecognized image volumes setting")
		}
	}
	return nil
}

func (ctr *container) setupSecretMounts(serverConfig *sconfig.Config, containerInfo storage.ContainerInfo, mountPoint string) []rspec.Mount {
	secretMounts := subscriptions.MountsWithUIDGID(
		containerInfo.MountLabel,
		containerInfo.RunDir,
		serverConfig.DefaultMountsFile,
		mountPoint,
		0,
		0,
		rootless.IsRootless(),
		ctr.DisableFips(),
	)
	for _, mount := range secretMounts {
		ctr.addMount(&mount)
	}
	return secretMounts
}

func (ctr *container) addOCIBindMounts(ctx context.Context, mountLabel string, serverConfig *sconfig.Config, maybeRelabel, idMapSupport, cgroup2RW bool) ([]oci.ContainerVolume, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	const superPrivilegedType = "spc_t"
	volumes := []oci.ContainerVolume{}
	containerConfig := ctr.Config()
	specgen := ctr.Spec()
	mounts := containerConfig.Mounts
	skipRelabel := false
	securityContext := containerConfig.Linux.SecurityContext
	bindMountPrefix := serverConfig.RuntimeConfig.BindMountPrefix

	if securityContext.SelinuxOptions == nil {
		securityContext.SelinuxOptions = &types.SELinuxOption{}
	}
	if securityContext.SelinuxOptions.Type == superPrivilegedType || // super privileged container
		(ctr.SandboxConfig().Linux != nil &&
			ctr.SandboxConfig().Linux.SecurityContext != nil &&
			ctr.SandboxConfig().Linux.SecurityContext.SelinuxOptions != nil &&
			ctr.SandboxConfig().Linux.SecurityContext.SelinuxOptions.Type == superPrivilegedType && // super privileged pod
			securityContext.SelinuxOptions.Type == "") {
		skipRelabel = true
	}

	// Sort mounts in number of parts. This ensures that high level mounts don't
	// shadow other mounts.
	sort.Sort(criOrderedMounts(mounts))

	// Add default mounts to the list
	defaultMounts := specgen.Mounts()
	specgen.ClearMounts()
	for _, m := range defaultMounts {
		// Lets handle the overridden mounts when adding to the OCI spec gen
		ctr.addMount(&m)
	}

	// Get mount info from system
	mountInfos, err := mount.GetMounts()
	if err != nil {
		return nil, err
	}

	// Handle mounts from container spec (CRI)
	for _, m := range mounts {
		dest := m.ContainerPath
		if dest == "" {
			return nil, fmt.Errorf("mount.ContainerPath is empty")
		}
		if m.HostPath == "" {
			return nil, fmt.Errorf("mount.HostPath is empty")
		}
		if m.HostPath == "/" && dest == "/" {
			log.Warnf(ctx, "Configuration specifies mounting host root to the container root.  This is dangerous (especially with privileged containers) and should be avoided.")
		}

		if isSubDirectoryOf(serverConfig.Root, m.HostPath) {
			log.Infof(ctx, "Mount propogration for the host path %s will be set to HostToContainer as it includes the container storage root", m.HostPath)
			m.Propagation = types.MountPropagation_PROPAGATION_HOST_TO_CONTAINER
		}

		src := filepath.Join(bindMountPrefix, m.HostPath)

		resolvedSrc, err := resolveSymbolicLink(bindMountPrefix, src)
		if err == nil {
			src = resolvedSrc
		} else {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to resolve symlink %q: %w", src, err)
			}
			for _, toReject := range serverConfig.AbsentMountSourcesToReject {
				if filepath.Clean(src) == toReject {
					// special-case /etc/hostname, as we don't want it to be created as a directory
					// This can cause issues with node reboot.
					return nil, fmt.Errorf("cannot mount %s: path does not exist and will cause issues as a directory", toReject)
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
					return nil, fmt.Errorf("failed to mkdir %s: %s", src, err)
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
				return nil, err
			}
			options = append(options, "rshared")
			if err := specgen.SetLinuxRootPropagation("rshared"); err != nil {
				return nil, err
			}
		case types.MountPropagation_PROPAGATION_HOST_TO_CONTAINER:
			if err := ensureSharedOrSlave(src, mountInfos); err != nil {
				return nil, err
			}
			options = append(options, "rslave")
			if specgen.Config.Linux.RootfsPropagation != "rshared" &&
				specgen.Config.Linux.RootfsPropagation != "rslave" {
				if err := specgen.SetLinuxRootPropagation("rslave"); err != nil {
					return nil, err
				}
			}
		default:
			log.Warnf(ctx, "Unknown propagation mode for hostPath %q", m.HostPath)
			options = append(options, "rprivate")
		}

		if m.SelinuxRelabel {
			if skipRelabel {
				log.Debugf(ctx, "Skipping relabel for %s because of super privileged container (type: spc_t)", src)
			} else if err := SecurityLabel(src, mountLabel, false, maybeRelabel); err != nil {
				return nil, err
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
			return nil, fmt.Errorf("idmap mounts specified but OCI runtime does not support them. Perhaps the OCI runtime is too old")
		}
		ctr.addMount(&rspec.Mount{
			Destination: dest,
			Type:        "bind",
			Source:      src,
			Options:     options,
			UIDMappings: uidMappings,
			GIDMappings: gidMappings,
		})
	}

	// Check for cgroup2RW
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
	ctr.addMount(&m)

	return volumes, nil
}

// systemd expects to have /run, /run/lock and /tmp on tmpfs
// It also expects to be able to write to /sys/fs/cgroup/systemd and /var/log/journal
func (ctr *container) setupSystemdMounts(containerInfo storage.ContainerInfo) error {
	if ctr.WillRunSystemd() {
		var err error
		containerInfo.ProcessLabel, err = selinux.InitLabel(containerInfo.ProcessLabel)
		if err != nil {
			return err
		}
		options := []string{"rw", "rprivate", "noexec", "nosuid", "nodev", "tmpcopyup"}
		destinations := []string{"/run", "/run/lock", "/tmp", "/var/log/journal"}
		for _, dest := range destinations {
			ctr.addMount(&rspec.Mount{
				Destination: dest,
				Type:        "tmpfs",
				Source:      "tmpfs",
				Options:     options,
			})
		}

		if node.CgroupIsV2() {
			ctr.addMount(&rspec.Mount{
				Destination: "/sys/fs/cgroup",
				Type:        "cgroup",
				Source:      "cgroup",
				Options:     []string{"private", "rw"},
			})
		} else {
			// If the /sys/fs/cgroup is bind mounted from the host,
			// then systemd-mode cgroup should be disabled
			// https://bugzilla.redhat.com/show_bug.cgi?id=2064741
			if !ctr.isBindMounted([]string{
				"/sys/fs/cgroup",
				"/sys/fs",
				"/sys"}) {
				ctr.addMount(&rspec.Mount{
					Destination: "/sys/fs/cgroup/systemd",
					Type:        "bind",
					Source:      "/sys/fs/cgroup/systemd",
					Options:     []string{"bind", "nodev", "noexec", "nosuid"},
				})
			}
			ctr.Spec().AddLinuxMaskedPaths(filepath.Join("/sys/fs/cgroup/systemd", "release_agent"))
		}
		ctr.Spec().AddProcessEnv("container", "crio")
	}
	return nil
}

func (c *container) isBindMounted(destinations []string) bool {
	for _, dest := range destinations {
		if mount, isPresent := c.mounts[dest]; isPresent {
			for _, option := range mount.Options {
				if option == "bind" || option == "rbind" {
					return true
				}
			}
		}
	}
	return false
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
