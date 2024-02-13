package server

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/containers/common/pkg/subscriptions"
	"github.com/containers/common/pkg/timezone"
	"github.com/containers/common/pkg/util"
	"github.com/containers/podman/v4/pkg/rootless"
	selinux "github.com/containers/podman/v4/pkg/selinux"
	cstorage "github.com/containers/storage"
	"github.com/containers/storage/pkg/idtools"
	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/config/device"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/config/rdt"
	ctrfactory "github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/linklogs"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/runtimehandlerhooks"
	"github.com/cri-o/cri-o/internal/storage"
	crioann "github.com/cri-o/cri-o/pkg/annotations"
	securejoin "github.com/cyphar/filepath-securejoin"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubeletTypes "k8s.io/kubelet/pkg/types"

	"github.com/intel/goresctrl/pkg/blockio"
)

const (
	cgroupSysFsPath        = "/sys/fs/cgroup"
	cgroupSysFsSystemdPath = "/sys/fs/cgroup/systemd"
)

// createContainerPlatform performs platform dependent intermediate steps before calling the container's oci.Runtime().CreateContainer()
func (s *Server) createContainerPlatform(ctx context.Context, container *oci.Container, cgroupParent string, idMappings *idtools.IDMappings) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	if idMappings != nil && !container.Spoofed() {
		rootPair := idMappings.RootPair()
		for _, path := range []string{container.BundlePath(), container.MountPoint()} {
			if err := makeAccessible(path, rootPair.UID, rootPair.GID, false); err != nil {
				return fmt.Errorf("cannot make %s accessible to %d:%d: %w", path, rootPair.UID, rootPair.GID, err)
			}
		}
		if err := makeMountsAccessible(rootPair.UID, rootPair.GID, container.Spec().Mounts); err != nil {
			return err
		}
	}
	return s.Runtime().CreateContainer(ctx, container, cgroupParent, false)
}

// makeAccessible changes the path permission and each parent directory to have --x--x--x
func makeAccessible(path string, uid, gid int, doChown bool) error {
	if doChown {
		if err := os.Chown(path, uid, gid); err != nil {
			return fmt.Errorf("cannot chown %s to %d:%d: %w", path, uid, gid, err)
		}
	}
	for ; path != "/"; path = filepath.Dir(path) {
		var st unix.Stat_t
		err := unix.Stat(path, &st)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if int(st.Uid) == uid && int(st.Gid) == gid {
			continue
		}
		perm := os.FileMode(st.Mode) & os.ModePerm
		if perm&0o111 != 0o111 {
			if err := os.Chmod(path, perm|0o111); err != nil {
				return err
			}
		}
	}
	return nil
}

// makeMountsAccessible makes sure all the mounts are accessible from the user namespace
func makeMountsAccessible(uid, gid int, mounts []rspec.Mount) error {
	for _, m := range mounts {
		if m.Type == "bind" || util.StringInSlice("bind", m.Options) {
			if err := makeAccessible(m.Source, uid, gid, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func toContainer(id uint32, idMap []idtools.IDMap) uint32 {
	hostID := int(id)
	if idMap == nil {
		return uint32(hostID)
	}
	for _, m := range idMap {
		if hostID >= m.HostID && hostID < m.HostID+m.Size {
			contID := m.ContainerID + (hostID - m.HostID)
			return uint32(contID)
		}
	}
	// If the ID cannot be mapped, it means the RunAsUser or RunAsGroup was not specified
	// so just use the original value.
	return id
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

	if sb.Annotations()[crioann.UsernsModeAnnotation] == "" {
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

func (s *Server) createSandboxContainer(ctx context.Context, ctr ctrfactory.Container, sb *sandbox.Sandbox) (cntr *oci.Container, retErr error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	// Helper function to get or create the security context
	getOrCreateSecurityContext := func(containerConfig *types.ContainerConfig) *types.LinuxContainerSecurityContext {
		if containerConfig.Linux == nil {
			containerConfig.Linux = &types.LinuxContainerConfig{}
		}
		securityContext := containerConfig.Linux.SecurityContext
		if securityContext == nil {
			securityContext = newLinuxContainerSecurityContext()
			containerConfig.Linux.SecurityContext = securityContext
		}
		return securityContext
	}
	var (
		err                        error
		containerConfig            = ctr.Config()
		containerID                = ctr.ID()
		containerName              = ctr.Name()
		containerConfigAnnotations = containerConfig.Annotations
		securityContext            = getOrCreateSecurityContext(containerConfig)
		specgen                    = ctr.Spec() // creates a spec Generator with the default spec.
		readOnlyRootfs             = ctr.ReadOnly(s.config.ReadOnly)
		metadata                   = containerConfig.Metadata
		userRequestedImage         string
		imgResult                  *storage.ImageResult
		labelOptions               []string
		containerIDMappings        *idtools.IDMappings
		idMappingOptions           *cstorage.IDMappingOptions
	)
	// TODO: factor generating/updating the spec into something other projects can vendor

	// eventually, we'd like to access all of these variables through the interface themselves, and do most
	// of the translation between CRI config -> oci/storage container in the container package

	// TODO: eventually, this should be in the container package, but it's going through a lot of churn
	// and SpecAddAnnotations is already being passed too many arguments
	// Filter early so any use of the annotations don't use the wrong values
	if err := s.FilterDisallowedAnnotations(sb.Annotations(), containerConfigAnnotations, sb.RuntimeHandler()); err != nil {
		return nil, err
	}

	if err := ctr.SetPrivileged(); err != nil {
		return nil, err
	}

	specgen.HostSpecific = true
	specgen.ClearProcessRlimits()

	for _, u := range s.config.Ulimits() {
		specgen.AddProcessRlimits(u.Name, u.Hard, u.Soft)
	}

	specgen.SetRootReadonly(readOnlyRootfs)

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
		ctr.AddMountsIfNotExistsInCRI(mounts, true, "tmpfs", "tmpfs", options)
	}

	userRequestedImage, err = ctr.UserRequestedImage()
	if err != nil {
		return nil, err
	}

	imgResult, err = s.getImageInfo(userRequestedImage)
	if err != nil {
		return nil, fmt.Errorf("failed to get image info for %s: %w", userRequestedImage, err)
	}
	// Get imageID that is requested later in container status
	imageID := imgResult.ID
	someRepoDigest := ""
	if len(imgResult.RepoDigests) > 0 {
		someRepoDigest = imgResult.RepoDigests[0]
	}

	labelOptions, err = ctr.SelinuxLabel(sb.ProcessLabel())
	if err != nil {
		return nil, err
	}

	containerIDMappings, err = s.getSandboxIDMappings(ctx, sb)
	if err != nil {
		return nil, err
	}

	if containerIDMappings != nil {
		idMappingOptions = &cstorage.IDMappingOptions{UIDMap: containerIDMappings.UIDs(), GIDMap: containerIDMappings.GIDs()}
	}

	s.resourceStore.SetStageForResource(ctx, containerName, "container storage creation")
	containerInfo, err := s.StorageRuntimeServer().CreateContainer(s.config.SystemContext,
		sb.Name(), sb.ID(),
		userRequestedImage, imageID,
		containerName, containerID,
		metadata.Name,
		metadata.Attempt,
		idMappingOptions,
		labelOptions,
		ctr.Privileged(),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			log.Infof(ctx, "CreateCtrLinux: deleting container %s from storage", containerInfo.ID)
			if err := s.StorageRuntimeServer().DeleteContainer(ctx, containerInfo.ID); err != nil {
				log.Warnf(ctx, "Failed to cleanup container directory: %v", err)
			}
		}
	}()

	mountLabel := containerInfo.MountLabel
	var processLabel string
	if !ctr.Privileged() {
		processLabel = containerInfo.ProcessLabel
	}
	if securityContext.NamespaceOptions == nil {
		securityContext.NamespaceOptions = &types.NamespaceOption{}
	}
	hostIPC := securityContext.NamespaceOptions.Ipc == types.NamespaceMode_NODE
	hostPID := securityContext.NamespaceOptions.Pid == types.NamespaceMode_NODE
	hostNet := securityContext.NamespaceOptions.Network == types.NamespaceMode_NODE

	// Don't use SELinux separation with Host Pid or IPC Namespace or privileged.
	if hostPID || hostIPC {
		processLabel, mountLabel = "", ""
	}

	if hostNet && s.config.RuntimeConfig.HostNetworkDisableSELinux {
		processLabel = ""
	}

	maybeRelabel, skipRelabel := processSELinuxRelabelOptions(sb, securityContext, ctr)

	cgroup2RW := node.CgroupIsV2() && sb.Annotations()[crioann.Cgroup2RWAnnotation] == "true"

	s.resourceStore.SetStageForResource(ctx, containerName, "container volume configuration")
	idMapSupport := s.Runtime().RuntimeSupportsIDMap(sb.RuntimeHandler())
	containerVolumes, ociMounts, err := ctr.AddOCIBindMounts(ctx, mountLabel, s.config.RuntimeConfig.BindMountPrefix, s.config.AbsentMountSourcesToReject, maybeRelabel, skipRelabel, cgroup2RW, idMapSupport, s.Config().Root)
	if err != nil {
		return nil, err
	}

	s.resourceStore.SetStageForResource(ctx, containerName, "container device creation")
	configuredDevices := s.config.Devices()

	privilegedWithoutHostDevices, err := s.Runtime().PrivilegedWithoutHostDevices(sb.RuntimeHandler())
	if err != nil {
		return nil, err
	}

	annotationDevices, err := device.DevicesFromAnnotation(sb.Annotations()[crioann.DevicesAnnotation], s.config.AllowedDevices)
	if err != nil {
		return nil, err
	}

	if err := ctr.SpecAddDevices(configuredDevices, annotationDevices, privilegedWithoutHostDevices, s.config.DeviceOwnershipFromSecurityContext); err != nil {
		return nil, err
	}

	s.resourceStore.SetStageForResource(ctx, containerName, "container storage start")
	mountPoint, err := s.StorageRuntimeServer().StartContainer(containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to mount container %s(%s): %w", containerName, containerID, err)
	}

	defer func() {
		if retErr != nil {
			log.Infof(ctx, "CreateCtrLinux: stopping storage container %s", containerID)
			if err := s.StorageRuntimeServer().StopContainer(ctx, containerID); err != nil {
				log.Warnf(ctx, "Couldn't stop storage container: %v: %v", containerID, err)
			}
		}
	}()

	s.resourceStore.SetStageForResource(ctx, containerName, "container spec configuration")

	labels := containerConfig.Labels

	if err := validateLabels(labels); err != nil {
		return nil, err
	}

	if err := s.configureAppArmorProfileAndBlockIO(ctx, sb, ctr, securityContext, specgen); err != nil {
		return nil, fmt.Errorf("failed to set apparmor  or block io: %w", err)
	}

	logPath, err := ctr.LogPath(sb.LogDir())
	if err != nil {
		return nil, err
	}

	specgen.SetProcessTerminal(containerConfig.Tty)
	if containerConfig.Tty {
		specgen.AddProcessEnv("TERM", "xterm")
	}

	linux := containerConfig.Linux
	if linux != nil {
		if err := s.configureLinuxResourcesAndSecurity(ctx, linux, ctr, securityContext, sb, specgen); err != nil {
			return nil, fmt.Errorf("failed to configure linux resources and security: %w", err)
		}
	}

	if err := ctr.AddUnifiedResourcesFromAnnotations(sb.Annotations()); err != nil {
		return nil, err
	}

	var nsTargetCtr *oci.Container
	if target := containerConfig.Linux.SecurityContext.NamespaceOptions.TargetId; target != "" {
		nsTargetCtr = s.GetContainer(ctx, target)
	}

	if err := ctr.SpecAddNamespaces(sb, nsTargetCtr, &s.config); err != nil {
		return nil, err
	}

	defer func() {
		if retErr != nil && ctr.PidNamespace() != nil {
			log.Infof(ctx, "CreateCtrLinux: clearing PID namespace for container %s", containerInfo.ID)
			if err := ctr.PidNamespace().Remove(); err != nil {
				log.Warnf(ctx, "Failed to remove PID namespace: %v", err)
			}
		}
	}()

	// If the sandbox is configured to run in the host network, do not create a new network namespace
	if hostNet {
		ctr.AddMountsIfNotExistsInCRI("/sys", true, "sysfs", "sysfs", []string{"nosuid", "noexec", "nodev", "ro"})
		ctr.AddMountsIfNotExistsInCRI(cgroupSysFsPath, true, "cgroup", "cgroup", []string{"nosuid", "noexec", "nodev", "relatime", "ro"})
	}

	if ctr.Privileged() {
		ctr.AddMountsIfNotExistsInCRI("/sys", false, "sysfs", "sysfs", []string{"nosuid", "noexec", "nodev", "rw", "rslave"})
		ctr.AddMountsIfNotExistsInCRI(cgroupSysFsPath, false, "cgroup", "cgroup", []string{"nosuid", "noexec", "nodev", "rw", "relatime", "rslave"})
	}

	containerImageConfig := containerInfo.Config
	if containerImageConfig == nil {
		return nil, err
	}

	if err := ctr.SpecSetProcessArgs(containerImageConfig); err != nil {
		return nil, err
	}

	// When running on cgroupv2, automatically add a cgroup namespace for not privileged containers.
	if !ctr.Privileged() && node.CgroupIsV2() {
		if err := specgen.AddOrReplaceLinuxNamespace(string(rspec.CgroupNamespace), ""); err != nil {
			return nil, err
		}
	}

	ctr.AddMountsIfNotExistsInCRI("/dev/shm", false, "bind", sb.ShmPath(), []string{"rw", "bind"})
	options := []string{"rw"}
	if readOnlyRootfs {
		options = []string{"ro"}
	}
	if sb.ResolvPath() != "" {
		if err := securityLabel(sb.ResolvPath(), mountLabel, false, false); err != nil {
			return nil, err
		}
		ctr.AddMountsIfNotExistsInCRI("/etc/resolv.conf", false, "bind", sb.ResolvPath(), append(options, []string{"bind", "nodev", "nosuid", "noexec"}...))
	}

	if sb.HostnamePath() != "" {
		if err := securityLabel(sb.HostnamePath(), mountLabel, false, false); err != nil {
			return nil, err
		}
		ctr.AddMountsIfNotExistsInCRI("/etc/hostname", false, "bind", sb.HostnamePath(), append(options, "bind"))
	}

	if sb.ContainerEnvPath() != "" {
		if err := securityLabel(sb.ContainerEnvPath(), mountLabel, false, false); err != nil {
			return nil, err
		}
		ctr.AddMountsIfNotExistsInCRI("/run/.containerenv", false, "bind", sb.ContainerEnvPath(), append(options, "bind"))
	}

	if hostNet {
		// Only bind mount for host netns and when CRI does not give us any hosts file
		ctr.AddMountsIfNotExistsInCRI("/etc/hosts", true, "bind", "/etc/hosts", append(options, "bind"))
	}

	if ctr.Privileged() {
		setOCIBindMountsPrivileged(specgen)
	}

	// Set hostname and add env for hostname
	specgen.SetHostname(sb.Hostname())
	specgen.AddProcessEnv("HOSTNAME", sb.Hostname())

	seccompRef := types.SecurityProfile_Unconfined.String()

	if err := s.FilterDisallowedAnnotations(sb.Annotations(), imgResult.Annotations, sb.RuntimeHandler()); err != nil {
		return nil, fmt.Errorf("filter image annotations: %w", err)
	}

	if !ctr.Privileged() {
		notifier, ref, err := s.config.Seccomp().Setup(
			ctx,
			s.config.SystemContext,
			s.seccompNotifierChan,
			containerID,
			metadata.Name,
			sb.Annotations(),
			imgResult.Annotations,
			specgen,
			securityContext.Seccomp,
		)
		if err != nil {
			return nil, fmt.Errorf("setup seccomp: %w", err)
		}
		if notifier != nil {
			s.seccompNotifiers.Store(containerID, notifier)
		}
		seccompRef = ref
	}

	// Get RDT class
	rdtClass, err := s.Config().Rdt().ContainerClassFromAnnotations(metadata.Name, containerConfig.Annotations, sb.Annotations())
	if err != nil {
		return nil, err
	}
	if rdtClass != "" {
		log.Debugf(ctx, "Setting RDT ClosID of container %s to %q", containerID, rdt.ResctrlPrefix+rdtClass)
		// TODO: patch runtime-tools to support setting ClosID via a helper func similar to SetLinuxIntelRdtL3CacheSchema()
		specgen.Config.Linux.IntelRdt = &rspec.LinuxIntelRdt{ClosID: rdt.ResctrlPrefix + rdtClass}
	}
	// compute the runtime path for a given container
	platform := containerInfo.Config.Platform.OS + "/" + containerInfo.Config.Platform.Architecture
	runtimePath, err := s.Runtime().PlatformRuntimePath(sb.RuntimeHandler(), platform)
	if err != nil {
		return nil, err
	}
	err = ctr.SpecAddAnnotations(ctx, sb, containerVolumes, mountPoint, containerImageConfig.Config.StopSignal, imgResult, s.config.CgroupManager().IsSystemd(), seccompRef, runtimePath)
	if err != nil {
		return nil, err
	}

	if err := s.config.Workloads.MutateSpecGivenAnnotations(metadata.Name, ctr.Spec(), sb.Annotations()); err != nil {
		return nil, err
	}

	// First add any configured environment variables from crio config.
	// They will get overridden if specified in the image or container config.
	specgen.AddMultipleProcessEnv(s.Config().DefaultEnv)

	// Add environment variables from image the CRI configuration
	envs := mergeEnvs(containerImageConfig, containerConfig.Envs)
	for _, e := range envs {
		parts := strings.SplitN(e, "=", 2)
		specgen.AddProcessEnv(parts[0], parts[1])
	}

	// Setup user and groups
	if linux != nil {
		if err := setupContainerUser(ctx, specgen, mountPoint, mountLabel, containerInfo.RunDir, securityContext, containerImageConfig); err != nil {
			return nil, err
		}
	}

	// Add image volumes
	volumeMounts, err := addImageVolumes(ctx, mountPoint, s, &containerInfo, mountLabel, specgen)
	if err != nil {
		return nil, err
	}

	// Set working directory
	// Pick it up from image config first and override if specified in CRI
	containerCwd := "/"
	imageCwd := containerImageConfig.Config.WorkingDir
	if imageCwd != "" {
		containerCwd = imageCwd
	}
	runtimeCwd := containerConfig.WorkingDir
	if runtimeCwd != "" {
		containerCwd = runtimeCwd
	}
	specgen.SetProcessCwd(containerCwd)
	if err := setupWorkingDirectory(mountPoint, mountLabel, containerCwd); err != nil {
		return nil, fmt.Errorf("failed to setup working directory: %w", err)
	}

	// Add secrets from the default and override mounts.conf files
	secretMounts := subscriptions.MountsWithUIDGID(
		mountLabel,
		containerInfo.RunDir,
		s.config.DefaultMountsFile,
		mountPoint,
		0,
		0,
		rootless.IsRootless(),
		ctr.DisableFips(),
	)

	mounts := []rspec.Mount{}
	mounts = append(mounts, ociMounts...)
	mounts = append(mounts, volumeMounts...)
	mounts = append(mounts, secretMounts...)

	sort.Sort(orderedMounts(mounts))

	for _, m := range mounts {
		rspecMount := rspec.Mount{
			Type:        "bind",
			Options:     append(m.Options, "bind"),
			Destination: m.Destination,
			Source:      m.Source,
			UIDMappings: m.UIDMappings,
			GIDMappings: m.GIDMappings,
		}
		ctr.SpecAddMount(rspecMount)
	}

	if ctr.WillRunSystemd() {
		processLabel, err = selinux.InitLabel(processLabel)
		if err != nil {
			return nil, err
		}
		setupSystemd(specgen.Mounts(), *specgen)
	}

	if s.ContainerServer.Hooks != nil {
		newAnnotations := map[string]string{}
		for key, value := range containerConfig.Annotations {
			newAnnotations[key] = value
		}
		for key, value := range sb.Annotations() {
			newAnnotations[key] = value
		}

		if _, err := s.ContainerServer.Hooks.Hooks(specgen.Config, newAnnotations, len(containerConfig.Mounts) > 0); err != nil {
			return nil, err
		}
	}

	// Set up pids limit if pids cgroup is mounted
	if node.CgroupHasPid() {
		specgen.SetLinuxResourcesPidsLimit(s.config.PidsLimit)
	}

	// by default, the root path is an empty string. set it now.
	specgen.SetRootPath(mountPoint)

	crioAnnotations := specgen.Config.Annotations

	criMetadata := &types.ContainerMetadata{
		Name:    metadata.Name,
		Attempt: metadata.Attempt,
	}
	ociContainer, err := oci.NewContainer(containerID, containerName, containerInfo.RunDir, logPath, labels, crioAnnotations, containerConfigAnnotations, userRequestedImage, imgResult.SomeNameOfThisImage, &imageID, someRepoDigest, criMetadata, sb.ID(), containerConfig.Tty, containerConfig.Stdin, containerConfig.StdinOnce, sb.RuntimeHandler(), containerInfo.Dir, time.Now(), containerImageConfig.Config.StopSignal)
	if err != nil {
		return nil, err
	}

	specgen.SetLinuxMountLabel(mountLabel)
	specgen.SetProcessSelinuxLabel(processLabel)

	ociContainer.AddManagedPIDNamespace(ctr.PidNamespace())

	ociContainer.SetIDMappings(containerIDMappings)
	var rootPair idtools.IDPair
	if containerIDMappings != nil {
		s.finalizeUserMapping(sb, specgen, containerIDMappings)

		for _, uidmap := range containerIDMappings.UIDs() {
			specgen.AddLinuxUIDMapping(uint32(uidmap.HostID), uint32(uidmap.ContainerID), uint32(uidmap.Size))
		}
		for _, gidmap := range containerIDMappings.GIDs() {
			specgen.AddLinuxGIDMapping(uint32(gidmap.HostID), uint32(gidmap.ContainerID), uint32(gidmap.Size))
		}

		rootPair = containerIDMappings.RootPair()

		pathsToChown := []string{mountPoint, containerInfo.RunDir}
		for _, m := range secretMounts {
			pathsToChown = append(pathsToChown, m.Source)
		}
		for _, path := range pathsToChown {
			if err := makeAccessible(path, rootPair.UID, rootPair.GID, true); err != nil {
				return nil, fmt.Errorf("cannot chown %s to %d:%d: %w", path, rootPair.UID, rootPair.GID, err)
			}
		}
	} else if err := specgen.RemoveLinuxNamespace(string(rspec.UserNamespace)); err != nil {
		return nil, err
	}
	if v := sb.Annotations()[crioann.UmaskAnnotation]; v != "" {
		umaskRegexp := regexp.MustCompile(`^[0-7]{1,4}$`)
		if !umaskRegexp.MatchString(v) {
			return nil, fmt.Errorf("invalid umask string %s", v)
		}
		decVal, err := strconv.ParseUint(sb.Annotations()[crioann.UmaskAnnotation], 8, 32)
		if err != nil {
			return nil, err
		}
		umask := uint32(decVal)
		specgen.Config.Process.User.Umask = &umask
	}

	if containerIDMappings == nil {
		rootPair = idtools.IDPair{UID: 0, GID: 0}
	}

	etc := filepath.Join(mountPoint, "/etc")
	// create the `/etc` folder only when it doesn't exist
	if _, err := os.Stat(etc); err != nil && os.IsNotExist(err) {
		if err := idtools.MkdirAllAndChown(etc, 0o755, rootPair); err != nil {
			return nil, fmt.Errorf("error creating mtab directory: %w", err)
		}
	}
	// add symlink /etc/mtab to /proc/mounts allow looking for mountfiles there in the container
	// compatible with Docker
	if err := os.Symlink("/proc/mounts", filepath.Join(etc, "mtab")); err != nil && !os.IsExist(err) {
		return nil, err
	}

	// Configure timezone for the container if it is set.
	if err := configureTimezone(s.Runtime().Timezone(), ociContainer.BundlePath(), mountPoint, mountLabel, etc, ociContainer.ID(), options, ctr); err != nil {
		return nil, fmt.Errorf("failed to configure timezone for container %s: %w", ociContainer.ID(), err)
	}

	if os.Getenv(rootlessEnvName) != "" {
		makeOCIConfigurationRootless(specgen)
	}

	hooks, err := runtimehandlerhooks.GetRuntimeHandlerHooks(ctx, &s.config, sb.RuntimeHandler(), sb.Annotations())
	if err != nil {
		return nil, fmt.Errorf("failed to get runtime handler %q hooks", sb.RuntimeHandler())
	}

	if err := s.nri.createContainer(ctx, specgen, sb, ociContainer); err != nil {
		return nil, err
	}

	defer func() {
		if retErr != nil {
			s.nri.undoCreateContainer(ctx, specgen, sb, ociContainer)
		}
	}()

	if hooks != nil {
		if err := hooks.PreCreate(ctx, specgen, sb, ociContainer); err != nil {
			return nil, fmt.Errorf("failed to run pre-create hook for container %q: %w", ociContainer.ID(), err)
		}
	}

	if emptyDirVolName, ok := sb.Annotations()[crioann.LinkLogsAnnotation]; ok {
		if err := linklogs.LinkContainerLogs(ctx, sb.Labels()[kubeletTypes.KubernetesPodUIDLabel], emptyDirVolName, containerID, metadata); err != nil {
			log.Warnf(ctx, "Failed to link container logs: %v", err)
		}
	}

	saveConfig := func(dir, fileName string) error {
		options := generate.ExportOptions{}
		return specgen.SaveToFile(filepath.Join(dir, fileName), options)
	}

	if err := saveConfig(containerInfo.Dir, "config.json"); err != nil {
		return nil, err
	}

	if err := saveConfig(containerInfo.RunDir, "config.json"); err != nil {
		return nil, err
	}

	ociContainer.SetSpec(specgen.Config)
	ociContainer.SetMountPoint(mountPoint)
	ociContainer.SetSeccompProfilePath(seccompRef)
	if runtimePath != "" {
		ociContainer.SetRuntimePathForPlatform(runtimePath)
	}

	for _, cv := range containerVolumes {
		ociContainer.AddVolume(cv)
	}

	return ociContainer, nil
}

func configureTimezone(tz, containerRunDir, mountPoint, mountLabel, etcPath, containerID string, options []string, ctr ctrfactory.Container) error {
	localTimePath, err := timezone.ConfigureContainerTimeZone(tz, containerRunDir, mountPoint, etcPath, containerID)
	if err != nil {
		return fmt.Errorf("setting timezone for container %s: %w", containerID, err)
	}
	if localTimePath != "" {
		if err := securityLabel(localTimePath, mountLabel, false, false); err != nil {
			return err
		}
		ctr.AddMountsIfNotExistsInCRI("/etc/localtime", false, "bind", localTimePath, append(options, []string{"bind", "nodev", "nosuid", "noexec"}...))
	}
	return nil
}

func setupWorkingDirectory(rootfs, mountLabel, containerCwd string) error {
	fp, err := securejoin.SecureJoin(rootfs, containerCwd)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(fp, 0o755); err != nil {
		return err
	}
	if mountLabel != "" {
		if err1 := securityLabel(fp, mountLabel, false, false); err1 != nil {
			return err1
		}
	}
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

// systemd expects to have /run, /run/lock and /tmp on tmpfs
// It also expects to be able to write to /sys/fs/cgroup/systemd and /var/log/journal
func setupSystemd(mounts []rspec.Mount, g generate.Generator) {
	mountSet := make(map[string]struct{})
	for _, m := range mounts {
		mountSet[m.Destination] = struct{}{}
	}
	options := []string{"rw", "rprivate", "noexec", "nosuid", "nodev"}
	for _, dest := range []string{"/run", "/run/lock"} {
		if _, mounted := mountSet[dest]; !mounted {
			tmpfsMnt := rspec.Mount{
				Destination: dest,
				Type:        "tmpfs",
				Source:      "tmpfs",
				Options:     append(options, "tmpcopyup"),
			}
			g.AddMount(tmpfsMnt)
		}
	}
	for _, dest := range []string{"/tmp", "/var/log/journal"} {
		if _, mounted := mountSet[dest]; !mounted {
			tmpfsMnt := rspec.Mount{
				Destination: dest,
				Type:        "tmpfs",
				Source:      "tmpfs",
				Options:     append(options, "tmpcopyup"),
			}
			g.AddMount(tmpfsMnt)
		}
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
		for _, gMount := range g.Mounts() {
			if _, mounted := mountSet[gMount.Destination]; !mounted && !isCgroupMount(&gMount) {
				systemdMnt := rspec.Mount{
					Destination: cgroupSysFsSystemdPath,
					Type:        "bind",
					Source:      cgroupSysFsSystemdPath,
					Options:     []string{"bind", "nodev", "noexec", "nosuid"},
				}
				g.AddMount(systemdMnt)
			}
		}
		g.AddLinuxMaskedPaths(filepath.Join(cgroupSysFsSystemdPath, "release_agent"))
	}
	g.AddProcessEnv("container", "crio")
}

func isCgroupMount(rMount *rspec.Mount) bool {
	return (rMount.Destination == cgroupSysFsPath || rMount.Destination == "/sys/fs" || rMount.Destination == "/sys") && isBindMount(rMount.Options)
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

func (s *Server) configureAppArmorProfileAndBlockIO(ctx context.Context, sb *sandbox.Sandbox, ctr ctrfactory.Container, securityContext *types.LinuxContainerSecurityContext, specgen *generate.Generator) error {
	if s.Config().AppArmor().IsEnabled() && !ctr.Privileged() {
		profile, err := s.Config().AppArmor().Apply(securityContext.ApparmorProfile)
		if err != nil {
			return fmt.Errorf("applying AppArmor profile to container %s: %w", ctr.ID(), err)
		}

		log.Debugf(ctx, "Applied AppArmor profile %s to container %s", profile, ctr.ID())
		specgen.SetProcessApparmorProfile(profile)
	}

	if s.Config().BlockIO().Enabled() {
		if blockioClass, err := blockio.ContainerClassFromAnnotations(ctr.Config().Metadata.Name, ctr.Config().Annotations, sb.Annotations()); blockioClass != "" && err == nil {
			if s.Config().BlockIO().ReloadRequired() {
				if err := s.Config().BlockIO().Reload(); err != nil {
					log.Warnf(ctx, "Reconfiguring blockio for container %s failed: %v", ctr.ID(), err)
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

func (s *Server) configureLinuxResourcesAndSecurity(ctx context.Context, linux *types.LinuxContainerConfig, ctr ctrfactory.Container, securityContext *types.LinuxContainerSecurityContext, sb *sandbox.Sandbox, specgen *generate.Generator) error {
	resources := linux.Resources
	if resources != nil {
		specgen.SetLinuxResourcesCPUPeriod(uint64(resources.CpuPeriod))
		specgen.SetLinuxResourcesCPUQuota(resources.CpuQuota)
		specgen.SetLinuxResourcesCPUShares(uint64(resources.CpuShares))

		memoryLimit := resources.MemoryLimitInBytes
		if memoryLimit != 0 {
			if err := cgmgr.VerifyMemoryIsEnough(memoryLimit); err != nil {
				return err
			}
			specgen.SetLinuxResourcesMemoryLimit(memoryLimit)
			if resources.MemorySwapLimitInBytes != 0 {
				if resources.MemorySwapLimitInBytes > 0 && resources.MemorySwapLimitInBytes < resources.MemoryLimitInBytes {
					return fmt.Errorf(
						"container %s create failed because memory swap limit (%d) cannot be lower than memory limit (%d)",
						ctr.ID(),
						resources.MemorySwapLimitInBytes,
						resources.MemoryLimitInBytes,
					)
				}
				memoryLimit = resources.MemorySwapLimitInBytes
			}
			// If node doesn't have memory swap, then skip setting
			// otherwise the container creation fails.
			if node.CgroupHasMemorySwap() {
				specgen.SetLinuxResourcesMemorySwap(memoryLimit)
			}
		}

		specgen.SetProcessOOMScoreAdj(int(resources.OomScoreAdj))
		specgen.SetLinuxResourcesCPUCpus(resources.CpusetCpus)
		specgen.SetLinuxResourcesCPUMems(resources.CpusetMems)

		// If the kernel has no support for hugetlb, silently ignore the limits
		if node.CgroupHasHugetlb() {
			hugepageLimits := resources.HugepageLimits
			for _, limit := range hugepageLimits {
				specgen.AddLinuxResourcesHugepageLimit(limit.PageSize, limit.Limit)
			}
		}

		if node.CgroupIsV2() && len(resources.Unified) != 0 {
			if specgen.Config.Linux.Resources.Unified == nil {
				specgen.Config.Linux.Resources.Unified = make(map[string]string, len(resources.Unified))
			}
			for key, value := range resources.Unified {
				specgen.Config.Linux.Resources.Unified[key] = value
			}
		}
	}

	specgen.SetLinuxCgroupsPath(s.Config().CgroupManager().ContainerCgroupPath(sb.CgroupParent(), ctr.ID()))

	if ctr.Privileged() {
		specgen.SetupPrivileged(true)
	} else {
		capabilities := securityContext.Capabilities
		if err := ctr.SpecSetupCapabilities(capabilities, s.Config().DefaultCapabilities, s.Config().AddInheritableCapabilities); err != nil {
			return err
		}
	}

	if securityContext.NoNewPrivs {
		const sysAdminCap = "CAP_SYS_ADMIN"
		for _, cap := range specgen.Config.Process.Capabilities.Bounding {
			if cap == sysAdminCap {
				log.Warnf(ctx, "Setting `noNewPrivileges` flag has no effect because container has %s capability", sysAdminCap)
			}
		}

		if ctr.Privileged() {
			log.Warnf(ctx, "Setting `noNewPrivileges` flag has no effect because container is privileged")
		}
	}

	specgen.SetProcessNoNewPrivileges(securityContext.NoNewPrivs)

	if !ctr.Privileged() {
		if securityContext.MaskedPaths != nil {
			for _, path := range securityContext.MaskedPaths {
				specgen.AddLinuxMaskedPaths(path)
			}
		}

		if securityContext.ReadonlyPaths != nil {
			for _, path := range securityContext.ReadonlyPaths {
				specgen.AddLinuxReadonlyPaths(path)
			}
		}
	}
	return nil
}
