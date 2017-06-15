package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/stringid"
	"github.com/docker/docker/pkg/symlink"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/pkg/annotations"
	"github.com/kubernetes-incubator/cri-o/server/apparmor"
	"github.com/kubernetes-incubator/cri-o/server/seccomp"
	"github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runc/libcontainer/devices"
	"github.com/opencontainers/runc/libcontainer/user"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/opencontainers/selinux/go-selinux/label"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

const (
	seccompUnconfined      = "unconfined"
	seccompRuntimeDefault  = "runtime/default"
	seccompLocalhostPrefix = "localhost/"
)

func addOCIBindMounts(sb *sandbox, containerConfig *pb.ContainerConfig, specgen *generate.Generator) error {
	mounts := containerConfig.GetMounts()
	for _, mount := range mounts {
		dest := mount.ContainerPath
		if dest == "" {
			return fmt.Errorf("Mount.ContainerPath is empty")
		}

		src := mount.HostPath
		if src == "" {
			return fmt.Errorf("Mount.HostPath is empty")
		}

		if _, err := os.Stat(src); err != nil && os.IsNotExist(err) {
			os.MkdirAll(src, 0644)
		}

		options := []string{"rw"}
		if mount.Readonly {
			options = []string{"ro"}
		}

		if mount.SelinuxRelabel {
			// Need a way in kubernetes to determine if the volume is shared or private
			if err := label.Relabel(src, sb.mountLabel, true); err != nil && err != syscall.ENOTSUP {
				return fmt.Errorf("relabel failed %s: %v", src, err)
			}
		}

		specgen.AddBindMount(src, dest, options)
	}

	return nil
}

func addDevices(sb *sandbox, containerConfig *pb.ContainerConfig, specgen *generate.Generator) error {
	sp := specgen.Spec()
	for _, device := range containerConfig.GetDevices() {
		dev, err := devices.DeviceFromPath(device.HostPath, device.Permissions)
		if err != nil {
			return fmt.Errorf("failed to add device: %v", err)
		}
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
	}
	return nil
}

// buildOCIProcessArgs build an OCI compatible process arguments slice.
func buildOCIProcessArgs(containerKubeConfig *pb.ContainerConfig, imageOCIConfig *v1.Image) ([]string, error) {
	//# Start the nginx container using the default command, but use custom
	//arguments (arg1 .. argN) for that command.
	//kubectl run nginx --image=nginx -- <arg1> <arg2> ... <argN>

	//# Start the nginx container using a different command and custom arguments.
	//kubectl run nginx --image=nginx --command -- <cmd> <arg1> ... <argN>

	kubeCommands := containerKubeConfig.Command
	kubeArgs := containerKubeConfig.Args

	// merge image config and kube config
	// same as docker does today...
	if imageOCIConfig != nil {
		if len(kubeCommands) == 0 {
			if len(kubeArgs) == 0 {
				kubeArgs = imageOCIConfig.Config.Cmd
			}
			if kubeCommands == nil {
				kubeCommands = imageOCIConfig.Config.Entrypoint
			}
		}
	}

	if len(kubeCommands) == 0 && len(kubeArgs) == 0 {
		return nil, fmt.Errorf("no command specified")
	}

	// create entrypoint and args
	var entrypoint string
	var args []string
	if len(kubeCommands) != 0 {
		entrypoint = kubeCommands[0]
		args = append(kubeCommands[1:], kubeArgs...)
	} else {
		entrypoint = kubeArgs[0]
		args = kubeArgs[1:]
	}

	processArgs := append([]string{entrypoint}, args...)

	logrus.Debugf("OCI process args %v", processArgs)

	return processArgs, nil
}

// setupContainerUser sets the UID, GID and supplemental groups in OCI runtime config
func setupContainerUser(specgen *generate.Generator, rootfs string, sc *pb.LinuxContainerSecurityContext, imageConfig *v1.Image) error {
	if sc != nil {
		containerUser := ""
		// Case 1: run as user is set by kubelet
		if sc.RunAsUser != nil {
			containerUser = strconv.FormatInt(sc.GetRunAsUser().Value, 10)
		} else {
			// Case 2: run as username is set by kubelet
			userName := sc.RunAsUsername
			if userName != "" {
				containerUser = userName
			} else {
				// Case 3: get user from image config
				if imageConfig != nil {
					imageUser := imageConfig.Config.User
					if imageUser != "" {
						containerUser = imageUser
					}
				}
			}
		}

		logrus.Debugf("CONTAINER USER: %+v", containerUser)

		// Add uid, gid and groups from user
		uid, gid, addGroups, err1 := getUserInfo(rootfs, containerUser)
		if err1 != nil {
			return err1
		}

		logrus.Debugf("UID: %v, GID: %v, Groups: %+v", uid, gid, addGroups)
		specgen.SetProcessUID(uid)
		specgen.SetProcessGID(gid)
		for _, group := range addGroups {
			specgen.AddProcessAdditionalGid(group)
		}

		// Add groups from CRI
		groups := sc.SupplementalGroups
		for _, group := range groups {
			specgen.AddProcessAdditionalGid(uint32(group))
		}
	}
	return nil
}

func hostNetwork(containerConfig *pb.ContainerConfig) bool {
	securityContext := containerConfig.GetLinux().GetSecurityContext()
	if securityContext == nil || securityContext.GetNamespaceOptions() == nil {
		return false
	}

	return securityContext.GetNamespaceOptions().HostNetwork
}

// ensureSaneLogPath is a hack to fix https://issues.k8s.io/44043 which causes
// logPath to be a broken symlink to some magical Docker path. Ideally we
// wouldn't have to deal with this, but until that issue is fixed we have to
// remove the path if it's a broken symlink.
func ensureSaneLogPath(logPath string) error {
	// If the path exists but the resolved path does not, then we have a broken
	// symlink and we need to remove it.
	fi, err := os.Lstat(logPath)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		// Non-existant files and non-symlinks aren't our problem.
		return nil
	}

	_, err = os.Stat(logPath)
	if os.IsNotExist(err) {
		err = os.RemoveAll(logPath)
		if err != nil {
			return fmt.Errorf("ensureSaneLogPath remove bad logPath: %s", err)
		}
	}
	return nil
}

// CreateContainer creates a new container in specified PodSandbox
func (s *Server) CreateContainer(ctx context.Context, req *pb.CreateContainerRequest) (res *pb.CreateContainerResponse, err error) {
	logrus.Debugf("CreateContainerRequest %+v", req)

	s.updateLock.RLock()
	defer s.updateLock.RUnlock()

	sbID := req.PodSandboxId
	if sbID == "" {
		return nil, fmt.Errorf("PodSandboxId should not be empty")
	}

	sandboxID, err := s.podIDIndex.Get(sbID)
	if err != nil {
		return nil, fmt.Errorf("PodSandbox with ID starting with %s not found: %v", sbID, err)
	}

	sb := s.getSandbox(sandboxID)
	if sb == nil {
		return nil, fmt.Errorf("specified sandbox not found: %s", sandboxID)
	}

	// The config of the container
	containerConfig := req.GetConfig()
	if containerConfig == nil {
		return nil, fmt.Errorf("CreateContainerRequest.ContainerConfig is nil")
	}

	name := containerConfig.GetMetadata().Name
	if name == "" {
		return nil, fmt.Errorf("CreateContainerRequest.ContainerConfig.Name is empty")
	}

	attempt := containerConfig.GetMetadata().Attempt
	containerID, containerName, err := s.generateContainerIDandName(sb.name, name, attempt)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			s.releaseContainerName(containerName)
		}
	}()

	container, err := s.createSandboxContainer(ctx, containerID, containerName, sb, req.GetSandboxConfig(), containerConfig)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err2 := s.storageRuntimeServer.DeleteContainer(containerID)
			if err2 != nil {
				logrus.Warnf("Failed to cleanup container directory: %v", err2)
			}
		}
	}()

	if err = s.runtime.CreateContainer(container, sb.cgroupParent); err != nil {
		return nil, err
	}

	if err = s.runtime.UpdateStatus(container); err != nil {
		return nil, err
	}

	s.addContainer(container)

	if err = s.ctrIDIndex.Add(containerID); err != nil {
		s.removeContainer(container)
		return nil, err
	}

	s.containerStateToDisk(container)

	resp := &pb.CreateContainerResponse{
		ContainerId: containerID,
	}

	logrus.Debugf("CreateContainerResponse: %+v", resp)
	return resp, nil
}

func (s *Server) createSandboxContainer(ctx context.Context, containerID string, containerName string, sb *sandbox, SandboxConfig *pb.PodSandboxConfig, containerConfig *pb.ContainerConfig) (*oci.Container, error) {
	if sb == nil {
		return nil, errors.New("createSandboxContainer needs a sandbox")
	}

	// TODO: simplify this function (cyclomatic complexity here is high)
	// TODO: factor generating/updating the spec into something other projects can vendor

	// creates a spec Generator with the default spec.
	specgen := generate.New()
	specgen.HostSpecific = true

	if err := addOCIBindMounts(sb, containerConfig, &specgen); err != nil {
		return nil, err
	}

	if err := addDevices(sb, containerConfig, &specgen); err != nil {
		return nil, err
	}

	labels := containerConfig.GetLabels()

	metadata := containerConfig.GetMetadata()

	kubeAnnotations := containerConfig.GetAnnotations()
	if kubeAnnotations != nil {
		for k, v := range kubeAnnotations {
			specgen.AddAnnotation(k, v)
		}
	}

	// set this container's apparmor profile if it is set by sandbox
	if s.appArmorEnabled {
		appArmorProfileName := s.getAppArmorProfileName(sb.annotations, metadata.Name)
		if appArmorProfileName != "" {
			// reload default apparmor profile if it is unloaded.
			if s.appArmorProfile == apparmor.DefaultApparmorProfile {
				if err := apparmor.EnsureDefaultApparmorProfile(); err != nil {
					return nil, err
				}
			}

			specgen.SetProcessApparmorProfile(appArmorProfileName)
		}
	}
	var readOnlyRootfs bool
	if containerConfig.GetLinux().GetSecurityContext() != nil {
		if containerConfig.GetLinux().GetSecurityContext().Privileged {
			specgen.SetupPrivileged(true)
		}

		if containerConfig.GetLinux().GetSecurityContext().ReadonlyRootfs {
			readOnlyRootfs = true
			specgen.SetRootReadonly(true)
		}
	}

	logPath := containerConfig.LogPath
	if logPath == "" {
		// TODO: Should we use sandboxConfig.GetLogDirectory() here?
		logPath = filepath.Join(sb.logDir, containerID+".log")
	}
	if !filepath.IsAbs(logPath) {
		// XXX: It's not really clear what this should be versus the sbox logDirectory.
		logrus.Warnf("requested logPath for ctr id %s is a relative path: %s", containerID, logPath)
		logPath = filepath.Join(sb.logDir, logPath)
	}

	// Handle https://issues.k8s.io/44043
	if err := ensureSaneLogPath(logPath); err != nil {
		return nil, err
	}

	logrus.WithFields(logrus.Fields{
		"sbox.logdir": sb.logDir,
		"ctr.logfile": containerConfig.LogPath,
		"log_path":    logPath,
	}).Debugf("setting container's log_path")

	specgen.SetProcessTerminal(containerConfig.Tty)

	linux := containerConfig.GetLinux()
	if linux != nil {
		resources := linux.GetResources()
		if resources != nil {
			cpuPeriod := resources.CpuPeriod
			if cpuPeriod != 0 {
				specgen.SetLinuxResourcesCPUPeriod(uint64(cpuPeriod))
			}

			cpuQuota := resources.CpuQuota
			if cpuQuota != 0 {
				specgen.SetLinuxResourcesCPUQuota(cpuQuota)
			}

			cpuShares := resources.CpuShares
			if cpuShares != 0 {
				specgen.SetLinuxResourcesCPUShares(uint64(cpuShares))
			}

			memoryLimit := resources.MemoryLimitInBytes
			if memoryLimit != 0 {
				specgen.SetLinuxResourcesMemoryLimit(uint64(memoryLimit))
			}

			oomScoreAdj := resources.OomScoreAdj
			specgen.SetLinuxResourcesOOMScoreAdj(int(oomScoreAdj))
		}

		if sb.cgroupParent != "" {
			if s.config.CgroupManager == "systemd" {
				cgPath := sb.cgroupParent + ":" + "crio" + ":" + containerID
				specgen.SetLinuxCgroupsPath(cgPath)
			} else {
				specgen.SetLinuxCgroupsPath(sb.cgroupParent + "/" + containerID)
			}
		}

		capabilities := linux.GetSecurityContext().GetCapabilities()
		toCAPPrefixed := func(cap string) string {
			if !strings.HasPrefix(strings.ToLower(cap), "cap_") {
				return "CAP_" + cap
			}
			return cap
		}
		if capabilities != nil {
			addCaps := capabilities.AddCapabilities
			if addCaps != nil {
				for _, cap := range addCaps {
					if err := specgen.AddProcessCapability(toCAPPrefixed(cap)); err != nil {
						return nil, err
					}
				}
			}

			dropCaps := capabilities.DropCapabilities
			if dropCaps != nil {
				for _, cap := range dropCaps {
					if err := specgen.DropProcessCapability(toCAPPrefixed(cap)); err != nil {
						logrus.Debugf("failed to drop cap %s: %v", toCAPPrefixed(cap), err)
					}
				}
			}
		}

		specgen.SetProcessSelinuxLabel(sb.processLabel)
		specgen.SetLinuxMountLabel(sb.mountLabel)

		if containerConfig.GetLinux().GetSecurityContext() != nil &&
			containerConfig.GetLinux().GetSecurityContext().Privileged == false {
			for _, mp := range []string{
				"/proc/kcore",
				"/proc/latency_stats",
				"/proc/timer_list",
				"/proc/timer_stats",
				"/proc/sched_debug",
				"/sys/firmware",
			} {
				specgen.AddLinuxMaskedPaths(mp)
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
		}
	}
	// Join the namespace paths for the pod sandbox container.
	podInfraState := s.runtime.ContainerStatus(sb.infraContainer)

	logrus.Debugf("pod container state %+v", podInfraState)

	ipcNsPath := fmt.Sprintf("/proc/%d/ns/ipc", podInfraState.Pid)
	if err := specgen.AddOrReplaceLinuxNamespace("ipc", ipcNsPath); err != nil {
		return nil, err
	}

	netNsPath := sb.netNsPath()
	if netNsPath == "" {
		// The sandbox does not have a permanent namespace,
		// it's on the host one.
		netNsPath = fmt.Sprintf("/proc/%d/ns/net", podInfraState.Pid)
	}

	if err := specgen.AddOrReplaceLinuxNamespace("network", netNsPath); err != nil {
		return nil, err
	}

	imageSpec := containerConfig.GetImage()
	if imageSpec == nil {
		return nil, fmt.Errorf("CreateContainerRequest.ContainerConfig.Image is nil")
	}

	image := imageSpec.Image
	if image == "" {
		return nil, fmt.Errorf("CreateContainerRequest.ContainerConfig.Image.Image is empty")
	}

	// bind mount the pod shm
	specgen.AddBindMount(sb.shmPath, "/dev/shm", []string{"rw"})

	options := []string{"rw"}
	if readOnlyRootfs {
		options = []string{"ro"}
	}
	if sb.resolvPath != "" {
		// bind mount the pod resolver file
		specgen.AddBindMount(sb.resolvPath, "/etc/resolv.conf", options)
	}

	// Bind mount /etc/hosts for host networking containers
	if hostNetwork(containerConfig) {
		specgen.AddBindMount("/etc/hosts", "/etc/hosts", options)
	}

	if sb.hostname != "" {
		specgen.SetHostname(sb.hostname)
	}

	specgen.AddAnnotation(annotations.Name, containerName)
	specgen.AddAnnotation(annotations.SandboxID, sb.id)
	specgen.AddAnnotation(annotations.SandboxName, sb.infraContainer.Name())
	specgen.AddAnnotation(annotations.ContainerType, annotations.ContainerTypeContainer)
	specgen.AddAnnotation(annotations.LogPath, logPath)
	specgen.AddAnnotation(annotations.TTY, fmt.Sprintf("%v", containerConfig.Tty))
	specgen.AddAnnotation(annotations.Stdin, fmt.Sprintf("%v", containerConfig.Stdin))
	specgen.AddAnnotation(annotations.StdinOnce, fmt.Sprintf("%v", containerConfig.StdinOnce))
	specgen.AddAnnotation(annotations.Image, image)

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

	if err = s.setupSeccomp(&specgen, containerName, sb.annotations); err != nil {
		return nil, err
	}

	metaname := metadata.Name
	attempt := metadata.Attempt
	containerInfo, err := s.storageRuntimeServer.CreateContainer(s.imageContext,
		sb.name, sb.id,
		image, image,
		containerName, containerID,
		metaname,
		attempt,
		sb.mountLabel,
		nil)
	if err != nil {
		return nil, err
	}

	mountPoint, err := s.storageRuntimeServer.StartContainer(containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to mount container %s(%s): %v", containerName, containerID, err)
	}

	containerImageConfig := containerInfo.Config
	if containerImageConfig == nil {
		return nil, fmt.Errorf("empty image config for %s", image)
	}

	if containerImageConfig.Config.StopSignal != "" {
		// this key is defined in image-spec conversion document at https://github.com/opencontainers/image-spec/pull/492/files#diff-8aafbe2c3690162540381b8cdb157112R57
		specgen.AddAnnotation("org.opencontainers.image.stopSignal", containerImageConfig.Config.StopSignal)
	}

	// TODO: volume handling in CRI-O
	//       right now, we do just an mkdir in the container rootfs because we
	//       know kube manages volumes its own way and we don't need to behave
	//       like docker.
	//       For instance gcr.io/k8s-testimages/redis:e2e now work with CRI-O
	for dest := range containerImageConfig.Config.Volumes {
		fp, err := symlink.FollowSymlinkInScope(filepath.Join(mountPoint, dest), mountPoint)
		if err != nil {
			return nil, err
		}
		os.MkdirAll(fp, 0644)
	}

	processArgs, err := buildOCIProcessArgs(containerConfig, containerImageConfig)
	if err != nil {
		return nil, err
	}
	specgen.SetProcessArgs(processArgs)

	// Add environment variables from CRI and image config
	envs := containerConfig.GetEnvs()
	if envs != nil {
		for _, item := range envs {
			key := item.Key
			value := item.Value
			if key == "" {
				continue
			}
			specgen.AddProcessEnv(key, value)
		}
	}
	if containerImageConfig != nil {
		for _, item := range containerImageConfig.Config.Env {
			parts := strings.SplitN(item, "=", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid env from image: %s", item)
			}

			if parts[0] == "" {
				continue
			}
			specgen.AddProcessEnv(parts[0], parts[1])
		}
	}

	// Set working directory
	// Pick it up from image config first and override if specified in CRI
	containerCwd := "/"
	if containerImageConfig != nil {
		imageCwd := containerImageConfig.Config.WorkingDir
		if imageCwd != "" {
			containerCwd = imageCwd
		}
	}
	runtimeCwd := containerConfig.WorkingDir
	if runtimeCwd != "" {
		containerCwd = runtimeCwd
	}
	specgen.SetProcessCwd(containerCwd)

	// Setup user and groups
	if linux != nil {
		if err = setupContainerUser(&specgen, mountPoint, linux.GetSecurityContext(), containerImageConfig); err != nil {
			return nil, err
		}
	}

	// by default, the root path is an empty string. set it now.
	specgen.SetRootPath(mountPoint)

	saveOptions := generate.ExportOptions{}
	if err = specgen.SaveToFile(filepath.Join(containerInfo.Dir, "config.json"), saveOptions); err != nil {
		return nil, err
	}
	if err = specgen.SaveToFile(filepath.Join(containerInfo.RunDir, "config.json"), saveOptions); err != nil {
		return nil, err
	}

	container, err := oci.NewContainer(containerID, containerName, containerInfo.RunDir, logPath, sb.netNs(), labels, kubeAnnotations, imageSpec, metadata, sb.id, containerConfig.Tty, containerConfig.Stdin, containerConfig.StdinOnce, sb.privileged, containerInfo.Dir, created, containerImageConfig.Config.StopSignal)
	if err != nil {
		return nil, err
	}

	return container, nil
}

func (s *Server) setupSeccomp(specgen *generate.Generator, cname string, sbAnnotations map[string]string) error {
	profile, ok := sbAnnotations["security.alpha.kubernetes.io/seccomp/container/"+cname]
	if !ok {
		profile, ok = sbAnnotations["security.alpha.kubernetes.io/seccomp/pod"]
		if !ok {
			// running w/o seccomp, aka unconfined
			profile = seccompUnconfined
		}
	}
	if !s.seccompEnabled {
		if profile != seccompUnconfined {
			return fmt.Errorf("seccomp is not enabled in your kernel, cannot run with a profile")
		}
		logrus.Warn("seccomp is not enabled in your kernel, running container without profile")
	}
	if profile == seccompUnconfined {
		// running w/o seccomp, aka unconfined
		specgen.Spec().Linux.Seccomp = nil
		return nil
	}
	if profile == seccompRuntimeDefault {
		return seccomp.LoadProfileFromStruct(s.seccompProfile, specgen)
	}
	if !strings.HasPrefix(profile, seccompLocalhostPrefix) {
		return fmt.Errorf("unknown seccomp profile option: %q", profile)
	}
	//file, err := ioutil.ReadFile(filepath.Join(s.seccompProfileRoot, strings.TrimPrefix(profile, seccompLocalhostPrefix)))
	//if err != nil {
	//return err
	//}
	// TODO(runcom): setup from provided node's seccomp profile
	// can't do this yet, see https://issues.k8s.io/36997
	return nil
}

func (s *Server) generateContainerIDandName(podName string, name string, attempt uint32) (string, string, error) {
	var (
		err error
		id  = stringid.GenerateNonCryptoID()
	)
	nameStr := fmt.Sprintf("%s-%s-%v", podName, name, attempt)
	if name == "infra" {
		nameStr = fmt.Sprintf("%s-%s", podName, name)
	}
	if name, err = s.reserveContainerName(id, nameStr); err != nil {
		return "", "", err
	}
	return id, name, err
}

// getAppArmorProfileName gets the profile name for the given container.
func (s *Server) getAppArmorProfileName(annotations map[string]string, ctrName string) string {
	profile := apparmor.GetProfileNameFromPodAnnotations(annotations, ctrName)

	if profile == "" {
		return ""
	}

	if profile == apparmor.ProfileRuntimeDefault {
		// If the value is runtime/default, then return default profile.
		return s.appArmorProfile
	}

	return strings.TrimPrefix(profile, apparmor.ProfileNamePrefix)
}

// openContainerFile opens a file inside a container rootfs safely
func openContainerFile(rootfs string, path string) (io.ReadCloser, error) {
	fp, err := symlink.FollowSymlinkInScope(filepath.Join(rootfs, path), rootfs)
	if err != nil {
		return nil, err
	}
	return os.Open(fp)
}

// getUserInfo returns UID, GID and additional groups for specified user
// by looking them up in /etc/passwd and /etc/group
func getUserInfo(rootfs string, userName string) (uint32, uint32, []uint32, error) {
	// We don't care if we can't open the file because
	// not all images will have these files
	passwdFile, err := openContainerFile(rootfs, "/etc/passwd")
	if err != nil {
		logrus.Warnf("Failed to open /etc/passwd: %v", err)
	} else {
		defer passwdFile.Close()
	}

	groupFile, err := openContainerFile(rootfs, "/etc/group")
	if err != nil {
		logrus.Warnf("Failed to open /etc/group: %v", err)
	} else {
		defer groupFile.Close()
	}

	execUser, err := user.GetExecUser(userName, nil, passwdFile, groupFile)
	if err != nil {
		return 0, 0, nil, err
	}

	uid := uint32(execUser.Uid)
	gid := uint32(execUser.Gid)
	var additionalGids []uint32
	for _, g := range execUser.Sgids {
		additionalGids = append(additionalGids, uint32(g))
	}

	return uid, gid, additionalGids, nil
}
