// +build linux

package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/containers/buildah/pkg/secrets"
	"github.com/containers/libpod/pkg/annotations"
	"github.com/containers/libpod/pkg/rootless"
	createconfig "github.com/containers/libpod/pkg/spec"
	"github.com/containers/storage/pkg/mount"
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/utils"
	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/devices"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// minMemoryLimit is the minimum memory that must be set for a container.
// A lower value would result in the container failing to start.
const minMemoryLimit = 12582912

// Copied from k8s.io/kubernetes/pkg/kubelet/kuberuntime/labels.go
const podTerminationGracePeriodLabel = "io.kubernetes.pod.terminationGracePeriod"

var (
	_cgroupv2HasHugetlbOnce sync.Once
	_cgroupv2HasHugetlb     bool
	_cgroupv2HasHugetlbErr  error
)

// cgroupv2HasHugetlb returns whether the hugetlb controller is present on
// cgroup v2.
func cgroupv2HasHugetlb() (bool, error) {
	_cgroupv2HasHugetlbOnce.Do(func() {
		controllers, err := ioutil.ReadFile("/sys/fs/cgroup/cgroup.controllers")
		if err != nil {
			_cgroupv2HasHugetlbErr = errors.Wrap(err, "read /sys/fs/cgroup/cgroup.controllers")
			return
		}
		_cgroupv2HasHugetlb = strings.Contains(string(controllers), "hugetlb")
	})
	return _cgroupv2HasHugetlb, _cgroupv2HasHugetlbErr
}

type configDevice struct {
	Device   rspec.LinuxDevice
	Resource rspec.LinuxDeviceCgroup
}

func findCgroupMountpoint(name string) error {
	// Set up pids limit if pids cgroup is mounted
	_, err := cgroups.FindCgroupMountpoint("", name)
	return err
}

func addDevicesPlatform(ctx context.Context, sb *sandbox.Sandbox, containerConfig *pb.ContainerConfig, privilegedWithoutHostDevices bool, specgen *generate.Generator) error {
	sp := specgen.Config
	if containerConfig.GetLinux().GetSecurityContext().GetPrivileged() && !privilegedWithoutHostDevices {
		hostDevices, err := devices.HostDevices()
		if err != nil {
			return err
		}
		for _, hostDevice := range hostDevices {
			rd := rspec.LinuxDevice{
				Path:  hostDevice.Path,
				Type:  string(hostDevice.Type),
				Major: hostDevice.Major,
				Minor: hostDevice.Minor,
				UID:   &hostDevice.Uid,
				GID:   &hostDevice.Gid,
			}
			if hostDevice.Major == 0 && hostDevice.Minor == 0 {
				// Invalid device, most likely a symbolic link, skip it.
				continue
			}
			specgen.AddDevice(rd)
		}
		sp.Linux.Resources.Devices = []rspec.LinuxDeviceCgroup{
			{
				Allow:  true,
				Access: "rwm",
			},
		}
	}

	for _, device := range containerConfig.GetDevices() {
		// pin the device to avoid using `device` within the range scope as
		// wrong function literal
		device := device

		// If we are privileged, we have access to devices on the host.
		// If the requested container path already exists on the host, the container won't see the expected host path.
		// Therefore, we must error out if the container path already exists
		privileged := containerConfig.GetLinux().GetSecurityContext() != nil && containerConfig.GetLinux().GetSecurityContext().GetPrivileged()
		if privileged && device.ContainerPath != device.HostPath {
			// we expect this to not exist
			_, err := os.Stat(device.ContainerPath)
			if err == nil {
				return errors.Errorf("privileged container was configured with a device container path that already exists on the host.")
			}
			if !os.IsNotExist(err) {
				return errors.Wrap(err, "error checking if container path exists on host")
			}
		}

		path, err := resolveSymbolicLink(device.HostPath, "/")
		if err != nil {
			return err
		}
		dev, err := devices.DeviceFromPath(path, device.Permissions)
		// if there was no error, return the device
		if err == nil {
			rd := rspec.LinuxDevice{
				Path:  device.ContainerPath,
				Type:  string(dev.Type),
				Major: dev.Major,
				Minor: dev.Minor,
				UID:   &dev.Uid,
				GID:   &dev.Gid,
			}
			specgen.AddDevice(rd)
			sp.Linux.Resources.Devices = append(sp.Linux.Resources.Devices, rspec.LinuxDeviceCgroup{
				Allow:  true,
				Type:   string(dev.Type),
				Major:  &dev.Major,
				Minor:  &dev.Minor,
				Access: dev.Permissions,
			})
			continue
		}
		// if the device is not a device node
		// try to see if it's a directory holding many devices
		if err == devices.ErrNotADevice {
			// check if it is a directory
			if e := utils.IsDirectory(path); e == nil {
				// mount the internal devices recursively
				// nolint: errcheck
				filepath.Walk(path, func(dpath string, f os.FileInfo, e error) error {
					if e != nil {
						log.Debugf(ctx, "addDevice walk: %v", e)
					}
					childDevice, e := devices.DeviceFromPath(dpath, device.Permissions)
					if e != nil {
						// ignore the device
						return nil
					}
					cPath := strings.Replace(dpath, path, device.ContainerPath, 1)
					rd := rspec.LinuxDevice{
						Path:  cPath,
						Type:  string(childDevice.Type),
						Major: childDevice.Major,
						Minor: childDevice.Minor,
						UID:   &childDevice.Uid,
						GID:   &childDevice.Gid,
					}
					specgen.AddDevice(rd)
					sp.Linux.Resources.Devices = append(sp.Linux.Resources.Devices, rspec.LinuxDeviceCgroup{
						Allow:  true,
						Type:   string(childDevice.Type),
						Major:  &childDevice.Major,
						Minor:  &childDevice.Minor,
						Access: childDevice.Permissions,
					})

					return nil
				})
			}
		}
	}
	return nil
}

// createContainerPlatform performs platform dependent intermediate steps before calling the container's oci.Runtime().CreateContainer()
func (s *Server) createContainerPlatform(container *oci.Container, cgroupParent string) error {
	if s.defaultIDMappings != nil && !s.defaultIDMappings.Empty() {
		rootPair := s.defaultIDMappings.RootPair()

		for _, path := range []string{container.BundlePath(), container.MountPoint()} {
			if err := os.Chown(path, rootPair.UID, rootPair.GID); err != nil {
				return errors.Wrapf(err, "cannot chown %s to %d:%d", path, rootPair.UID, rootPair.GID)
			}
			if err := makeAccessible(path, rootPair.UID, rootPair.GID); err != nil {
				return errors.Wrapf(err, "cannot make %s accessible to %d:%d", path, rootPair.UID, rootPair.GID)
			}
		}
	}
	return s.Runtime().CreateContainer(container, cgroupParent)
}

// makeAccessible changes the path permission and each parent directory to have --x--x--x
func makeAccessible(path string, uid, gid int) error {
	for ; path != "/"; path = filepath.Dir(path) {
		st, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if int(st.Sys().(*syscall.Stat_t).Uid) == uid && int(st.Sys().(*syscall.Stat_t).Gid) == gid {
			continue
		}
		if st.Mode()&0111 != 0111 {
			if err := os.Chmod(path, st.Mode()|0111); err != nil {
				return err
			}
		}
	}
	return nil
}

// nolint:gocyclo
func (s *Server) createSandboxContainer(ctx context.Context, containerID, containerName string, sb *sandbox.Sandbox, sandboxConfig *pb.PodSandboxConfig, containerConfig *pb.ContainerConfig) (*oci.Container, error) {
	if sb == nil {
		return nil, errors.New("createSandboxContainer needs a sandbox")
	}

	// TODO: simplify this function (cyclomatic complexity here is high)
	// TODO: factor generating/updating the spec into something other projects can vendor

	// creates a spec Generator with the default spec.
	specgen, err := generate.New("linux")
	if err != nil {
		return nil, err
	}
	specgen.HostSpecific = true
	specgen.ClearProcessRlimits()

	ulimits, err := getUlimitsFromConfig(&s.config)
	if err != nil {
		return nil, err
	}
	for _, u := range ulimits {
		specgen.AddProcessRlimits(u.name, u.hard, u.soft)
	}

	readOnlyRootfs := s.config.ReadOnly

	var privileged bool
	if containerConfig.GetLinux().GetSecurityContext() != nil {
		if containerConfig.GetLinux().GetSecurityContext().GetPrivileged() {
			privileged = true
		}
		if privileged {
			if !sandboxConfig.GetLinux().GetSecurityContext().GetPrivileged() {
				return nil, errors.New("no privileged container allowed in sandbox")
			}
		}
		if containerConfig.GetLinux().GetSecurityContext().GetReadonlyRootfs() {
			readOnlyRootfs = true
		}
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
		for target, mode := range mounts {
			if !isInCRIMounts(target, containerConfig.GetMounts()) {
				mnt := rspec.Mount{
					Destination: target,
					Type:        "tmpfs",
					Source:      "tmpfs",
					Options:     append(options, mode),
				}
				specgen.AddMount(mnt)
			}
		}
	}

	imageSpec := containerConfig.GetImage()
	if imageSpec == nil {
		return nil, fmt.Errorf("CreateContainerRequest.ContainerConfig.Image is nil")
	}

	image := imageSpec.Image
	if image == "" {
		return nil, fmt.Errorf("CreateContainerRequest.ContainerConfig.Image.Image is empty")
	}
	images, err := s.StorageImageServer().ResolveNames(s.config.SystemContext, image)
	if err != nil {
		if err == storage.ErrCannotParseImageID {
			images = append(images, image)
		} else {
			return nil, err
		}
	}

	// Get imageName and imageRef that are later requested in container status
	var (
		imgResult    *storage.ImageResult
		imgResultErr error
	)
	for _, img := range images {
		imgResult, imgResultErr = s.StorageImageServer().ImageStatus(s.config.SystemContext, img)
		if imgResultErr == nil {
			break
		}
	}
	if imgResultErr != nil {
		return nil, imgResultErr
	}

	imageName := imgResult.Name
	imageRef := imgResult.ID
	if len(imgResult.RepoDigests) > 0 {
		imageRef = imgResult.RepoDigests[0]
	}

	specgen.AddAnnotation(annotations.Image, image)
	specgen.AddAnnotation(annotations.ImageName, imageName)
	specgen.AddAnnotation(annotations.ImageRef, imageRef)

	selinuxConfig := containerConfig.GetLinux().GetSecurityContext().GetSelinuxOptions()
	var labelOptions []string
	if selinuxConfig == nil {
		labelOptions, err = label.DupSecOpt(sb.ProcessLabel())
		if err != nil {
			return nil, err
		}
	} else {
		labelOptions = getLabelOptions(selinuxConfig)
	}

	containerIDMappings := s.defaultIDMappings
	metadata := containerConfig.GetMetadata()

	containerInfo, err := s.StorageRuntimeServer().CreateContainer(s.config.SystemContext,
		sb.Name(), sb.ID(),
		image, imgResult.ID,
		containerName, containerID,
		metadata.Name,
		metadata.Attempt,
		containerIDMappings,
		labelOptions)
	if err != nil {
		return nil, err
	}
	mountLabel := containerInfo.MountLabel
	var processLabel string
	if !privileged {
		processLabel = containerInfo.ProcessLabel
	}
	hostIPC := containerConfig.GetLinux().GetSecurityContext().GetNamespaceOptions().GetIpc() == pb.NamespaceMode_NODE
	hostPID := containerConfig.GetLinux().GetSecurityContext().GetNamespaceOptions().GetPid() == pb.NamespaceMode_NODE
	hostNet := containerConfig.GetLinux().GetSecurityContext().GetNamespaceOptions().GetNetwork() == pb.NamespaceMode_NODE

	// Don't use SELinux separation with Host Pid or IPC Namespace or privileged.
	if hostPID || hostIPC {
		processLabel, mountLabel = "", ""
	}

	if hostNet {
		processLabel = ""
	}

	defer func() {
		if err != nil {
			err2 := s.StorageRuntimeServer().DeleteContainer(containerInfo.ID)
			if err2 != nil {
				log.Warnf(ctx, "Failed to cleanup container directory: %v", err2)
			}
		}
	}()
	specgen.SetLinuxMountLabel(mountLabel)
	specgen.SetProcessSelinuxLabel(processLabel)

	containerVolumes, ociMounts, err := addOCIBindMounts(ctx, mountLabel, containerConfig, &specgen, s.config.RuntimeConfig.BindMountPrefix)
	if err != nil {
		return nil, err
	}

	volumesJSON, err := json.Marshal(containerVolumes)
	if err != nil {
		return nil, err
	}
	specgen.AddAnnotation(annotations.Volumes, string(volumesJSON))

	configuredDevices, err := getDevicesFromConfig(ctx, &s.config)
	if err != nil {
		return nil, err
	}

	for i := range configuredDevices {
		d := &configuredDevices[i]

		specgen.AddDevice(d.Device)
		specgen.AddLinuxResourcesDevice(d.Resource.Allow, d.Resource.Type, d.Resource.Major, d.Resource.Minor, d.Resource.Access)
	}

	privilegedWithoutHostDevices, err := s.Runtime().PrivilegedWithoutHostDevices(sb.RuntimeHandler())
	if err != nil {
		return nil, err
	}

	if err := addDevices(ctx, sb, containerConfig, privilegedWithoutHostDevices, &specgen); err != nil {
		return nil, err
	}

	labels := containerConfig.GetLabels()

	if err := validateLabels(labels); err != nil {
		return nil, err
	}

	kubeAnnotations := containerConfig.GetAnnotations()
	for k, v := range kubeAnnotations {
		specgen.AddAnnotation(k, v)
	}
	for k, v := range labels {
		specgen.AddAnnotation(k, v)
	}

	// set this container's apparmor profile if it is set by sandbox
	if s.Config().AppArmor().IsEnabled() && !privileged {
		profile, err := s.Config().AppArmor().Apply(
			containerConfig.GetLinux().GetSecurityContext().GetApparmorProfile(),
		)
		if err != nil {
			return nil, errors.Wrapf(err, "applying apparmor profile to container %s", containerID)
		}

		log.Debugf(ctx, "Applied AppArmor profile %s to container %s", profile, containerID)
		specgen.SetProcessApparmorProfile(profile)
	}

	sboxLogDir := sandboxConfig.GetLogDirectory()
	if sboxLogDir == "" {
		sboxLogDir = sb.LogDir()
	}

	logPath := containerConfig.GetLogPath()
	if logPath == "" {
		logPath = filepath.Join(sboxLogDir, containerID+".log")
	} else {
		logPath = filepath.Join(sboxLogDir, logPath)
	}

	// Handle https://issues.k8s.io/44043
	if err := ensureSaneLogPath(logPath); err != nil {
		return nil, err
	}

	log.Debugf(ctx, "setting container's log_path = %s, sbox.logdir = %s, ctr.logfile = %s",
		sboxLogDir, containerConfig.GetLogPath(), logPath,
	)

	specgen.SetProcessTerminal(containerConfig.Tty)
	if containerConfig.Tty {
		specgen.AddProcessEnv("TERM", "xterm")
	}

	linux := containerConfig.GetLinux()
	if linux != nil {
		resources := linux.GetResources()
		if resources != nil {
			specgen.SetLinuxResourcesCPUPeriod(uint64(resources.GetCpuPeriod()))
			specgen.SetLinuxResourcesCPUQuota(resources.GetCpuQuota())
			specgen.SetLinuxResourcesCPUShares(uint64(resources.GetCpuShares()))

			memoryLimit := resources.GetMemoryLimitInBytes()
			if memoryLimit != 0 && memoryLimit < minMemoryLimit {
				return nil, fmt.Errorf("set memory limit %v too low; should be at least %v", memoryLimit, minMemoryLimit)
			}
			specgen.SetLinuxResourcesMemoryLimit(memoryLimit)

			specgen.SetProcessOOMScoreAdj(int(resources.GetOomScoreAdj()))
			specgen.SetLinuxResourcesCPUCpus(resources.GetCpusetCpus())
			specgen.SetLinuxResourcesCPUMems(resources.GetCpusetMems())

			supportsHugetlb := true
			if cgroups.IsCgroup2UnifiedMode() {
				supportsHugetlb, err = cgroupv2HasHugetlb()
				if err != nil {
					return nil, err
				}
			}

			// If the kernel has no support for hugetlb, silently ignore the limits
			if supportsHugetlb {
				hugepageLimits := resources.GetHugepageLimits()
				for _, limit := range hugepageLimits {
					specgen.AddLinuxResourcesHugepageLimit(limit.PageSize, limit.Limit)
				}
			}
		}

		var cgPath string
		parent := defaultCgroupfsParent
		useSystemd := s.config.CgroupManager == oci.SystemdCgroupsManager
		if useSystemd {
			parent = defaultSystemdParent
		}
		if sb.CgroupParent() != "" {
			parent = sb.CgroupParent()
		}
		if useSystemd {
			cgPath = parent + ":" + scopePrefix + ":" + containerID
		} else {
			cgPath = filepath.Join(parent, scopePrefix+"-"+containerID)
		}
		specgen.SetLinuxCgroupsPath(cgPath)

		if t, ok := kubeAnnotations[podTerminationGracePeriodLabel]; ok {
			// currently only supported by systemd, see
			// https://github.com/opencontainers/runc/pull/2224
			if useSystemd {
				specgen.AddAnnotation("org.systemd.property.TimeoutStopUSec",
					"uint64 "+t+"000000") // sec to usec
			}
		}

		if privileged {
			specgen.SetupPrivileged(true)
		} else {
			capabilities := linux.GetSecurityContext().GetCapabilities()
			// Ensure we don't get a nil pointer error if the config
			// doesn't set any capabilities
			if capabilities == nil {
				capabilities = &pb.Capability{}
			}
			// Clear default capabilities from spec
			specgen.ClearProcessCapabilities()
			capabilities.AddCapabilities = append(capabilities.AddCapabilities, s.config.DefaultCapabilities...)
			err = setupCapabilities(&specgen, capabilities)
			if err != nil {
				return nil, err
			}
		}
		specgen.SetProcessNoNewPrivileges(linux.GetSecurityContext().GetNoNewPrivs())

		if containerConfig.GetLinux().GetSecurityContext() != nil &&
			!containerConfig.GetLinux().GetSecurityContext().Privileged {
			// TODO(runcom): have just one of this var at the top of the function
			securityContext := containerConfig.GetLinux().GetSecurityContext()
			for _, mp := range []string{
				"/proc/acpi",
				"/proc/kcore",
				"/proc/keys",
				"/proc/latency_stats",
				"/proc/timer_list",
				"/proc/timer_stats",
				"/proc/sched_debug",
				"/proc/scsi",
				"/sys/firmware",
			} {
				specgen.AddLinuxMaskedPaths(mp)
			}
			if securityContext.GetMaskedPaths() != nil {
				specgen.Config.Linux.MaskedPaths = nil
				for _, path := range securityContext.GetMaskedPaths() {
					specgen.AddLinuxMaskedPaths(path)
				}
			}

			for _, rp := range []string{
				"/proc/asound",
				"/proc/bus",
				"/proc/fs",
				"/proc/irq",
				"/proc/sys",
				"/proc/sysrq-trigger",
			} {
				specgen.AddLinuxReadonlyPaths(rp)
			}
			if securityContext.GetReadonlyPaths() != nil {
				specgen.Config.Linux.ReadonlyPaths = nil
				for _, path := range securityContext.GetReadonlyPaths() {
					specgen.AddLinuxReadonlyPaths(path)
				}
			}
		}
	}

	// Join the namespace paths for the pod sandbox container.
	if err := configureGeneratorGivenNamespacePaths(sb.NamespacePaths(), specgen); err != nil {
		return nil, errors.Wrap(err, "failed to configure namespaces in container create")
	}

	if containerConfig.GetLinux().GetSecurityContext().GetNamespaceOptions().GetPid() == pb.NamespaceMode_NODE {
		// kubernetes PodSpec specify to use Host PID namespace
		if err := specgen.RemoveLinuxNamespace(string(rspec.PIDNamespace)); err != nil {
			return nil, err
		}
	} else if containerConfig.GetLinux().GetSecurityContext().GetNamespaceOptions().GetPid() == pb.NamespaceMode_POD {
		infra := sb.InfraContainer()
		if infra == nil {
			return nil, errors.New("PID namespace requested, but sandbox has no infra container")
		}

		// share Pod PID namespace
		// SEE NOTE ABOVE
		pidNsPath := fmt.Sprintf("/proc/%d/ns/pid", infra.State().Pid)
		if err := specgen.AddOrReplaceLinuxNamespace(string(rspec.PIDNamespace), pidNsPath); err != nil {
			return nil, err
		}
	}

	// If the sandbox is configured to run in the host network, do not create a new network namespace
	if sb.HostNetwork() {
		if err := specgen.RemoveLinuxNamespace(string(rspec.NetworkNamespace)); err != nil {
			return nil, err
		}

		if !isInCRIMounts("/sys", containerConfig.GetMounts()) {
			specgen.RemoveMount("/sys")
			specgen.RemoveMount("/sys/fs/cgroup")
			sysMnt := rspec.Mount{
				Destination: "/sys",
				Type:        "sysfs",
				Source:      "sysfs",
				Options:     []string{"nosuid", "noexec", "nodev", "ro"},
			}
			specgen.AddMount(sysMnt)
		}
	}

	if privileged {
		specgen.RemoveMount("/sys")
		specgen.RemoveMount("/sys/fs/cgroup")

		sysMnt := rspec.Mount{
			Destination: "/sys",
			Type:        "sysfs",
			Source:      "sysfs",
			Options:     []string{"nosuid", "noexec", "nodev", "rw"},
		}
		specgen.AddMount(sysMnt)

		cgroupMnt := rspec.Mount{
			Destination: "/sys/fs/cgroup",
			Type:        "cgroup",
			Source:      "cgroup",
			Options:     []string{"nosuid", "noexec", "nodev", "rw", "relatime"},
		}
		specgen.AddMount(cgroupMnt)
	}

	// When running on cgroupv2, automatically add a cgroup namespace for not privileged containers.
	if !privileged && cgroups.IsCgroup2UnifiedMode() {
		if err := specgen.AddOrReplaceLinuxNamespace(string(rspec.CgroupNamespace), ""); err != nil {
			return nil, err
		}
	}

	for idx, ip := range sb.IPs() {
		specgen.AddAnnotation(fmt.Sprintf("%s.%d", annotations.IP, idx), ip)
	}

	// Remove the default /dev/shm mount to ensure we overwrite it
	specgen.RemoveMount("/dev/shm")

	mnt := rspec.Mount{
		Type:        "bind",
		Source:      sb.ShmPath(),
		Destination: "/dev/shm",
		Options:     []string{"rw", "bind"},
	}
	// bind mount the pod shm
	specgen.AddMount(mnt)

	options := []string{"rw"}
	if readOnlyRootfs {
		options = []string{"ro"}
	}
	if sb.ResolvPath() != "" {
		if err := securityLabel(sb.ResolvPath(), mountLabel, false); err != nil {
			return nil, err
		}

		mnt = rspec.Mount{
			Type:        "bind",
			Source:      sb.ResolvPath(),
			Destination: "/etc/resolv.conf",
			Options:     []string{"bind", "nodev", "nosuid", "noexec"},
		}
		// bind mount the pod resolver file
		specgen.AddMount(mnt)
	}

	if sb.HostnamePath() != "" {
		if err := securityLabel(sb.HostnamePath(), mountLabel, false); err != nil {
			return nil, err
		}

		mnt = rspec.Mount{
			Type:        "bind",
			Source:      sb.HostnamePath(),
			Destination: "/etc/hostname",
			Options:     append(options, "bind"),
		}
		specgen.AddMount(mnt)
	}

	if !isInCRIMounts("/etc/hosts", containerConfig.GetMounts()) && hostNetwork(containerConfig) {
		// Only bind mount for host netns and when CRI does not give us any hosts file
		mnt = rspec.Mount{
			Type:        "bind",
			Source:      "/etc/hosts",
			Destination: "/etc/hosts",
			Options:     append(options, "bind"),
		}
		specgen.AddMount(mnt)
	}

	if privileged {
		setOCIBindMountsPrivileged(&specgen)
	}

	// Set hostname and add env for hostname
	specgen.SetHostname(sb.Hostname())
	specgen.AddProcessEnv("HOSTNAME", sb.Hostname())

	specgen.AddAnnotation(annotations.Name, containerName)
	specgen.AddAnnotation(annotations.ContainerID, containerID)
	specgen.AddAnnotation(annotations.SandboxID, sb.ID())
	specgen.AddAnnotation(annotations.SandboxName, sb.Name())
	specgen.AddAnnotation(annotations.ContainerType, annotations.ContainerTypeContainer)
	specgen.AddAnnotation(annotations.LogPath, logPath)
	specgen.AddAnnotation(annotations.TTY, fmt.Sprintf("%v", containerConfig.Tty))
	specgen.AddAnnotation(annotations.Stdin, fmt.Sprintf("%v", containerConfig.Stdin))
	specgen.AddAnnotation(annotations.StdinOnce, fmt.Sprintf("%v", containerConfig.StdinOnce))
	specgen.AddAnnotation(annotations.ResolvPath, sb.ResolvPath())
	specgen.AddAnnotation(annotations.ContainerManager, lib.ContainerManagerCRIO)

	created := time.Now()
	specgen.AddAnnotation(annotations.Created, created.Format(time.RFC3339Nano))

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	specgen.AddAnnotation(annotations.Metadata, string(metadataJSON))

	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, err
	}
	specgen.AddAnnotation(annotations.Labels, string(labelsJSON))

	kubeAnnotationsJSON, err := json.Marshal(kubeAnnotations)
	if err != nil {
		return nil, err
	}
	specgen.AddAnnotation(annotations.Annotations, string(kubeAnnotationsJSON))

	spp := containerConfig.GetLinux().GetSecurityContext().GetSeccompProfilePath()
	if !privileged {
		if err := s.setupSeccomp(ctx, &specgen, spp); err != nil {
			return nil, err
		}
	}
	specgen.AddAnnotation(annotations.SeccompProfilePath, spp)

	mountPoint, err := s.StorageRuntimeServer().StartContainer(containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to mount container %s(%s): %v", containerName, containerID, err)
	}
	specgen.AddAnnotation(annotations.MountPoint, mountPoint)

	containerImageConfig := containerInfo.Config
	if containerImageConfig == nil {
		err = fmt.Errorf("empty image config for %s", image)
		return nil, err
	}

	if containerImageConfig.Config.StopSignal != "" {
		// this key is defined in image-spec conversion document at https://github.com/opencontainers/image-spec/pull/492/files#diff-8aafbe2c3690162540381b8cdb157112R57
		specgen.AddAnnotation("org.opencontainers.image.stopSignal", containerImageConfig.Config.StopSignal)
	}

	// Add environment variables from image the CRI configuration
	envs := mergeEnvs(containerImageConfig, containerConfig.GetEnvs())
	for _, e := range envs {
		parts := strings.SplitN(e, "=", 2)
		specgen.AddProcessEnv(parts[0], parts[1])
	}

	// Setup user and groups
	if linux != nil {
		if err := setupContainerUser(ctx, &specgen, mountPoint, mountLabel, containerInfo.RunDir, linux.GetSecurityContext(), containerImageConfig); err != nil {
			return nil, err
		}
	}

	// Add image volumes
	volumeMounts, err := addImageVolumes(ctx, mountPoint, s, &containerInfo, mountLabel, &specgen)
	if err != nil {
		return nil, err
	}

	processArgs, err := buildOCIProcessArgs(ctx, containerConfig, containerImageConfig)
	if err != nil {
		return nil, err
	}
	specgen.SetProcessArgs(processArgs)

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
		if err1 := s.StorageRuntimeServer().StopContainer(containerID); err1 != nil {
			return nil, fmt.Errorf("can't umount container after cwd error %v: %v", err, err1)
		}
		return nil, err
	}

	var secretMounts []rspec.Mount
	if len(s.config.DefaultMounts) > 0 {
		// This option has been deprecated, once it is removed in the later versions, delete the server/secrets.go file as well
		log.Warnf(ctx, "--default-mounts has been deprecated and will be removed in future versions. Add mounts to either %q or %q", secrets.DefaultMountsFile, secrets.OverrideMountsFile)
		var err error
		secretMounts, err = addSecretsBindMounts(ctx, mountLabel, containerInfo.RunDir, s.config.DefaultMounts, specgen)
		if err != nil {
			return nil, fmt.Errorf("failed to mount secrets: %v", err)
		}
	}
	// Check for FIPS_DISABLE label in the pod config
	disableFips := false
	if value, ok := sandboxConfig.GetLabels()["FIPS_DISABLE"]; ok && value == "true" {
		disableFips = true
	}
	// Add secrets from the default and override mounts.conf files
	secretMounts = append(secretMounts, secrets.SecretMounts(mountLabel, containerInfo.RunDir, s.config.DefaultMountsFile, rootless.IsRootless(), disableFips)...)

	mounts := []rspec.Mount{}
	mounts = append(mounts, ociMounts...)
	mounts = append(mounts, volumeMounts...)
	mounts = append(mounts, secretMounts...)

	sort.Sort(orderedMounts(mounts))

	for _, m := range mounts {
		mnt = rspec.Mount{
			Type:        "bind",
			Source:      m.Source,
			Destination: m.Destination,
			Options:     append(m.Options, "bind"),
		}
		specgen.AddMount(mnt)
	}

	newAnnotations := map[string]string{}
	for key, value := range containerConfig.GetAnnotations() {
		newAnnotations[key] = value
	}
	for key, value := range sb.Annotations() {
		newAnnotations[key] = value
	}
	if s.ContainerServer.Hooks != nil {
		if _, err := s.ContainerServer.Hooks.Hooks(specgen.Config, newAnnotations, len(containerConfig.GetMounts()) > 0); err != nil {
			return nil, err
		}
	}

	// Set up pids limit if pids cgroup is mounted
	if findCgroupMountpoint("pids") == nil {
		specgen.SetLinuxResourcesPidsLimit(s.config.PidsLimit)
	}

	// by default, the root path is an empty string. set it now.
	specgen.SetRootPath(mountPoint)

	crioAnnotations := specgen.Config.Annotations

	container, err := oci.NewContainer(containerID, containerName, containerInfo.RunDir, logPath, labels, crioAnnotations, kubeAnnotations, image, imageName, imageRef, metadata, sb.ID(), containerConfig.Tty, containerConfig.Stdin, containerConfig.StdinOnce, sb.Privileged(), sb.RuntimeHandler(), containerInfo.Dir, created, containerImageConfig.Config.StopSignal)
	if err != nil {
		return nil, err
	}

	container.SetIDMappings(containerIDMappings)
	if s.defaultIDMappings != nil && !s.defaultIDMappings.Empty() {
		for _, uidmap := range s.defaultIDMappings.UIDs() {
			specgen.AddLinuxUIDMapping(uint32(uidmap.HostID), uint32(uidmap.ContainerID), uint32(uidmap.Size))
		}
		for _, gidmap := range s.defaultIDMappings.GIDs() {
			specgen.AddLinuxGIDMapping(uint32(gidmap.HostID), uint32(gidmap.ContainerID), uint32(gidmap.Size))
		}
	} else if err := specgen.RemoveLinuxNamespace(string(rspec.UserNamespace)); err != nil {
		return nil, err
	}

	if os.Getenv("_CRIO_ROOTLESS") != "" {
		makeOCIConfigurationRootless(&specgen)
	}

	saveOptions := generate.ExportOptions{}
	if err := specgen.SaveToFile(filepath.Join(containerInfo.Dir, "config.json"), saveOptions); err != nil {
		return nil, err
	}
	if err := specgen.SaveToFile(filepath.Join(containerInfo.RunDir, "config.json"), saveOptions); err != nil {
		return nil, err
	}

	container.SetSpec(specgen.Config)
	container.SetMountPoint(mountPoint)
	container.SetSeccompProfilePath(spp)

	for _, cv := range containerVolumes {
		container.AddVolume(cv)
	}

	return container, nil
}

func setupWorkingDirectory(rootfs, mountLabel, containerCwd string) error {
	fp, err := securejoin.SecureJoin(rootfs, containerCwd)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(fp, 0755); err != nil {
		return err
	}
	if mountLabel != "" {
		if err1 := securityLabel(fp, mountLabel, false); err1 != nil {
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

func addOCIBindMounts(ctx context.Context, mountLabel string, containerConfig *pb.ContainerConfig, specgen *generate.Generator, bindMountPrefix string) ([]oci.ContainerVolume, []rspec.Mount, error) {
	volumes := []oci.ContainerVolume{}
	ociMounts := []rspec.Mount{}
	mounts := containerConfig.GetMounts()

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

	for _, m := range mounts {
		dest := m.GetContainerPath()
		if dest == "" {
			return nil, nil, fmt.Errorf("mount.ContainerPath is empty")
		}

		if m.HostPath == "" {
			return nil, nil, fmt.Errorf("mount.HostPath is empty")
		}
		src := filepath.Join(bindMountPrefix, m.GetHostPath())

		resolvedSrc, err := resolveSymbolicLink(src, bindMountPrefix)
		if err == nil {
			src = resolvedSrc
		} else {
			if !os.IsNotExist(err) {
				return nil, nil, fmt.Errorf("failed to resolve symlink %q: %v", src, err)
			} else if err = os.MkdirAll(src, 0755); err != nil {
				return nil, nil, fmt.Errorf("failed to mkdir %s: %s", src, err)
			}
		}

		options := []string{"rw"}
		if m.Readonly {
			options = []string{"ro"}
		}
		options = append(options, "rbind")

		// mount propagation
		mountInfos, err := mount.GetMounts()
		if err != nil {
			return nil, nil, err
		}
		switch m.GetPropagation() {
		case pb.MountPropagation_PROPAGATION_PRIVATE:
			options = append(options, "rprivate")
			// Since default root propagation in runc is rprivate ignore
			// setting the root propagation
		case pb.MountPropagation_PROPAGATION_BIDIRECTIONAL:
			if err := ensureShared(src, mountInfos); err != nil {
				return nil, nil, err
			}
			options = append(options, "rshared")
			if err := specgen.SetLinuxRootPropagation("rshared"); err != nil {
				return nil, nil, err
			}
		case pb.MountPropagation_PROPAGATION_HOST_TO_CONTAINER:
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
			log.Warnf(ctx, "unknown propagation mode for hostPath %q", m.HostPath)
			options = append(options, "rprivate")
		}

		if m.SelinuxRelabel {
			if err := securityLabel(src, mountLabel, false); err != nil {
				return nil, nil, err
			}
		}

		volumes = append(volumes, oci.ContainerVolume{
			ContainerPath: dest,
			HostPath:      src,
			Readonly:      m.Readonly,
		})

		ociMounts = append(ociMounts, rspec.Mount{
			Source:      src,
			Destination: dest,
			Options:     options,
		})
	}

	if _, mountSys := mountSet["/sys"]; !mountSys {
		m := rspec.Mount{
			Destination: "/sys/fs/cgroup",
			Type:        "cgroup",
			Source:      "cgroup",
			Options:     []string{"nosuid", "noexec", "nodev", "relatime", "ro"},
		}
		specgen.AddMount(m)
	}

	return volumes, ociMounts, nil
}

func getDevicesFromConfig(ctx context.Context, config *libconfig.Config) ([]configDevice, error) {
	linuxdevs := make([]configDevice, 0, len(config.RuntimeConfig.AdditionalDevices))

	for _, d := range config.RuntimeConfig.AdditionalDevices {
		src, dst, permissions, err := createconfig.ParseDevice(d)
		if err != nil {
			return nil, err
		}

		log.Debugf(ctx, "adding device src=%s dst=%s mode=%s", src, dst, permissions)

		dev, err := devices.DeviceFromPath(src, permissions)
		if err != nil {
			return nil, errors.Wrapf(err, "%s is not a valid device", src)
		}

		dev.Path = dst

		linuxdevs = append(linuxdevs,
			configDevice{
				Device: rspec.LinuxDevice{
					Path:     dev.Path,
					Type:     string(dev.Type),
					Major:    dev.Major,
					Minor:    dev.Minor,
					FileMode: &dev.FileMode,
					UID:      &dev.Uid,
					GID:      &dev.Gid,
				},
				Resource: rspec.LinuxDeviceCgroup{
					Allow:  true,
					Type:   string(dev.Type),
					Major:  &dev.Major,
					Minor:  &dev.Minor,
					Access: permissions,
				},
			})
	}

	return linuxdevs, nil
}
