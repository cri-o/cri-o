// +build linux

package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containers/libpod/pkg/annotations"
	"github.com/containers/libpod/pkg/cgroups"
	"github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/lib/config"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/pkg/log"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runc/libcontainer/cgroups/systemd"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/leaky"
	"k8s.io/kubernetes/pkg/kubelet/types"
)

const cgroupMemorySubsystemMountPathV1 = "/sys/fs/cgroup/memory"
const cgroupMemorySubsystemMountPathV2 = "/sys/fs/cgroup"

func (s *Server) runPodSandbox(ctx context.Context, req *pb.RunPodSandboxRequest) (resp *pb.RunPodSandboxResponse, err error) {
	s.updateLock.RLock()
	defer s.updateLock.RUnlock()

	if req.GetConfig().GetMetadata() == nil {
		return nil, fmt.Errorf("CreateContainerRequest.ContainerConfig.Metadata is nil")
	}

	pathsToChown := []string{}

	// we need to fill in the container name, as it is not present in the request. Luckily, it is a constant.
	log.Infof(ctx, "attempting to run pod sandbox with infra container: %s%s", translateLabelsToDescription(req.GetConfig().GetLabels()), leaky.PodInfraContainerName)
	var processLabel, mountLabel, resolvPath string
	// process req.Name
	kubeName := req.GetConfig().GetMetadata().GetName()
	if kubeName == "" {
		return nil, fmt.Errorf("PodSandboxConfig.Name should not be empty")
	}

	namespace := req.GetConfig().GetMetadata().GetNamespace()
	attempt := req.GetConfig().GetMetadata().GetAttempt()

	id, name, err := s.ReservePodIDAndName(req.GetConfig())
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			s.ReleasePodName(name)
		}
	}()

	containerName, err := s.ReserveSandboxContainerIDAndName(req.GetConfig())
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			s.ReleaseContainerName(containerName)
		}
	}()

	var labelOptions []string
	securityContext := req.GetConfig().GetLinux().GetSecurityContext()
	selinuxConfig := securityContext.GetSelinuxOptions()
	if selinuxConfig != nil {
		labelOptions = getLabelOptions(selinuxConfig)
	}
	podContainer, err := s.StorageRuntimeServer().CreatePodSandbox(s.systemContext,
		name, id,
		s.config.PauseImage,
		s.config.PauseImageAuthFile,
		"",
		containerName,
		req.GetConfig().GetMetadata().GetName(),
		req.GetConfig().GetMetadata().GetUid(),
		namespace,
		attempt,
		s.defaultIDMappings,
		labelOptions)
	mountLabel = podContainer.MountLabel
	processLabel = podContainer.ProcessLabel

	if errors.Cause(err) == storage.ErrDuplicateName {
		return nil, fmt.Errorf("pod sandbox with name %q already exists", name)
	}
	if err != nil {
		return nil, fmt.Errorf("error creating pod sandbox with name %q: %v", name, err)
	}
	defer func() {
		if err != nil {
			if err2 := s.StorageRuntimeServer().RemovePodSandbox(id); err2 != nil {
				log.Warnf(ctx, "couldn't cleanup pod sandbox %q: %v", id, err2)
			}
		}
	}()

	// TODO: factor generating/updating the spec into something other projects can vendor

	// creates a spec Generator with the default spec.
	g, err := generate.New("linux")
	if err != nil {
		return nil, err
	}
	g.HostSpecific = true
	g.ClearProcessRlimits()

	ulimits, err := getUlimitsFromConfig(&s.config)
	if err != nil {
		return nil, err
	}
	for _, u := range ulimits {
		g.AddProcessRlimits(u.name, u.hard, u.soft)
	}

	// setup defaults for the pod sandbox
	g.SetRootReadonly(true)

	pauseCommand, err := PauseCommand(s.Config(), podContainer.Config)
	if err != nil {
		return nil, err
	}
	g.SetProcessArgs(pauseCommand)

	// set DNS options
	if req.GetConfig().GetDnsConfig() != nil {
		dnsServers := req.GetConfig().GetDnsConfig().Servers
		dnsSearches := req.GetConfig().GetDnsConfig().Searches
		dnsOptions := req.GetConfig().GetDnsConfig().Options
		resolvPath = fmt.Sprintf("%s/resolv.conf", podContainer.RunDir)
		err = parseDNSOptions(dnsServers, dnsSearches, dnsOptions, resolvPath)
		if err != nil {
			err1 := removeFile(resolvPath)
			if err1 != nil {
				err = err1
				return nil, fmt.Errorf("%v; failed to remove %s: %v", err, resolvPath, err1)
			}
			return nil, err
		}
		if err := label.Relabel(resolvPath, mountLabel, false); err != nil && err != unix.ENOTSUP {
			return nil, err
		}
		mnt := runtimespec.Mount{
			Type:        "bind",
			Source:      resolvPath,
			Destination: "/etc/resolv.conf",
			Options:     []string{"ro", "bind", "nodev", "nosuid", "noexec"},
		}
		pathsToChown = append(pathsToChown, resolvPath)
		g.AddMount(mnt)
	}

	// add metadata
	metadata := req.GetConfig().GetMetadata()
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	// add labels
	labels := req.GetConfig().GetLabels()

	if err := validateLabels(labels); err != nil {
		return nil, err
	}

	// Add special container name label for the infra container
	labelsJSON := []byte{}
	if labels != nil {
		labels[types.KubernetesContainerNameLabel] = leaky.PodInfraContainerName
		labelsJSON, err = json.Marshal(labels)
		if err != nil {
			return nil, err
		}
	}

	// add annotations
	kubeAnnotations := req.GetConfig().GetAnnotations()
	kubeAnnotationsJSON, err := json.Marshal(kubeAnnotations)
	if err != nil {
		return nil, err
	}

	// set log directory
	logDir := req.GetConfig().GetLogDirectory()
	if logDir == "" {
		logDir = filepath.Join(s.config.LogDir, id)
	}
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, err
	}
	// This should always be absolute from k8s.
	if !filepath.IsAbs(logDir) {
		return nil, fmt.Errorf("requested logDir for sbox id %s is a relative path: %s", id, logDir)
	}

	privileged := s.privilegedSandbox(req)

	// Add capabilities from crio.conf if default_capabilities is defined
	capabilities := &pb.Capability{}
	if s.config.DefaultCapabilities != nil {
		g.ClearProcessCapabilities()
		capabilities.AddCapabilities = append(capabilities.AddCapabilities, s.config.DefaultCapabilities...)
	}
	if err := setupCapabilities(&g, capabilities); err != nil {
		return nil, err
	}

	nsOptsJSON, err := json.Marshal(securityContext.GetNamespaceOptions())
	if err != nil {
		return nil, err
	}

	hostIPC := securityContext.GetNamespaceOptions().GetIpc() == pb.NamespaceMode_NODE
	hostPID := securityContext.GetNamespaceOptions().GetPid() == pb.NamespaceMode_NODE

	// Don't use SELinux separation with Host Pid or IPC Namespace or privileged.
	if hostPID || hostIPC {
		processLabel, mountLabel = "", ""
	}
	g.SetProcessSelinuxLabel(processLabel)
	g.SetLinuxMountLabel(mountLabel)

	// Remove the default /dev/shm mount to ensure we overwrite it
	g.RemoveMount("/dev/shm")

	// create shm mount for the pod containers.
	var shmPath string
	if hostIPC {
		shmPath = "/dev/shm"
	} else {
		shmPath, err = setupShm(podContainer.RunDir, mountLabel)
		if err != nil {
			return nil, err
		}
		pathsToChown = append(pathsToChown, shmPath)
		defer func() {
			if err != nil {
				if err2 := unix.Unmount(shmPath, unix.MNT_DETACH); err2 != nil {
					log.Warnf(ctx, "failed to unmount shm for pod: %v", err2)
				}
			}
		}()
	}

	mnt := runtimespec.Mount{
		Type:        "bind",
		Source:      shmPath,
		Destination: "/dev/shm",
		Options:     []string{"rw", "bind"},
	}
	// bind mount the pod shm
	g.AddMount(mnt)

	err = s.setPodSandboxMountLabel(id, mountLabel)
	if err != nil {
		return nil, err
	}

	if err := s.CtrIDIndex().Add(id); err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			if err2 := s.CtrIDIndex().Delete(id); err2 != nil {
				log.Warnf(ctx, "couldn't delete ctr id %s from idIndex", id)
			}
		}
	}()

	// set log path inside log directory
	logPath := filepath.Join(logDir, id+".log")

	// Handle https://issues.k8s.io/44043
	if err := ensureSaneLogPath(logPath); err != nil {
		return nil, err
	}

	hostNetwork := securityContext.GetNamespaceOptions().GetNetwork() == pb.NamespaceMode_NODE

	hostname, err := getHostname(id, req.GetConfig().Hostname, hostNetwork)
	if err != nil {
		return nil, err
	}
	g.SetHostname(hostname)

	// validate the runtime handler
	runtimeHandler, err := s.runtimeHandler(req)
	if err != nil {
		return nil, err
	}

	g.AddAnnotation(annotations.Metadata, string(metadataJSON))
	g.AddAnnotation(annotations.Labels, string(labelsJSON))
	g.AddAnnotation(annotations.Annotations, string(kubeAnnotationsJSON))
	g.AddAnnotation(annotations.LogPath, logPath)
	g.AddAnnotation(annotations.Name, name)
	g.AddAnnotation(annotations.Namespace, namespace)
	g.AddAnnotation(annotations.ContainerType, annotations.ContainerTypeSandbox)
	g.AddAnnotation(annotations.SandboxID, id)
	g.AddAnnotation(annotations.ContainerName, containerName)
	g.AddAnnotation(annotations.ContainerID, id)
	g.AddAnnotation(annotations.ShmPath, shmPath)
	g.AddAnnotation(annotations.PrivilegedRuntime, fmt.Sprintf("%v", privileged))
	g.AddAnnotation(annotations.RuntimeHandler, runtimeHandler)
	g.AddAnnotation(annotations.ResolvPath, resolvPath)
	g.AddAnnotation(annotations.HostName, hostname)
	g.AddAnnotation(annotations.NamespaceOptions, string(nsOptsJSON))
	g.AddAnnotation(annotations.KubeName, kubeName)
	g.AddAnnotation(annotations.HostNetwork, fmt.Sprintf("%v", hostNetwork))
	if podContainer.Config.Config.StopSignal != "" {
		// this key is defined in image-spec conversion document at https://github.com/opencontainers/image-spec/pull/492/files#diff-8aafbe2c3690162540381b8cdb157112R57
		g.AddAnnotation("org.opencontainers.image.stopSignal", podContainer.Config.Config.StopSignal)
	}

	created := time.Now()
	g.AddAnnotation(annotations.Created, created.Format(time.RFC3339Nano))

	portMappings := convertPortMappings(req.GetConfig().GetPortMappings())
	portMappingsJSON, err := json.Marshal(portMappings)
	if err != nil {
		return nil, err
	}
	g.AddAnnotation(annotations.PortMappings, string(portMappingsJSON))

	parent := ""
	cgroupv2, err := cgroups.IsCgroup2UnifiedMode()
	if err != nil {
		return nil, err
	}
	if cgroupv2 {
		parent = cgroupMemorySubsystemMountPathV2
	} else {
		parent = cgroupMemorySubsystemMountPathV1
	}

	cgroupParent, err := AddCgroupAnnotation(ctx, g, parent, s.config.CgroupManager, req.GetConfig().GetLinux().GetCgroupParent(), id)
	if err != nil {
		return nil, err
	}

	if s.defaultIDMappings != nil && !s.defaultIDMappings.Empty() {
		if err := g.AddOrReplaceLinuxNamespace(spec.UserNamespace, ""); err != nil {
			return nil, errors.Wrapf(err, "add or replace linux namespace")
		}
		for _, uidmap := range s.defaultIDMappings.UIDs() {
			g.AddLinuxUIDMapping(uint32(uidmap.HostID), uint32(uidmap.ContainerID), uint32(uidmap.Size))
		}
		for _, gidmap := range s.defaultIDMappings.GIDs() {
			g.AddLinuxGIDMapping(uint32(gidmap.HostID), uint32(gidmap.ContainerID), uint32(gidmap.Size))
		}
	}

	sb, err := sandbox.New(id, namespace, name, kubeName, logDir, labels, kubeAnnotations, processLabel, mountLabel, metadata, shmPath, cgroupParent, privileged, runtimeHandler, resolvPath, hostname, portMappings, hostNetwork)
	if err != nil {
		return nil, err
	}

	if err := s.addSandbox(sb); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			if err := s.removeSandbox(id); err != nil {
				log.Warnf(ctx, "could not remove pod sandbox: %v", err)
			}
		}
	}()

	if err := s.PodIDIndex().Add(id); err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			if err := s.PodIDIndex().Delete(id); err != nil {
				log.Warnf(ctx, "couldn't delete pod id %s from idIndex", id)
			}
		}
	}()

	for k, v := range kubeAnnotations {
		g.AddAnnotation(k, v)
	}
	for k, v := range labels {
		g.AddAnnotation(k, v)
	}

	// Add default sysctls given in crio.conf
	for _, defaultSysctl := range s.config.DefaultSysctls {
		split := strings.SplitN(defaultSysctl, "=", 2)
		if len(split) == 2 {
			if err := validateSysctl(split[0], hostNetwork, hostIPC); err != nil {
				log.Warnf(ctx, "sysctl not valid %q: %v - skipping...", defaultSysctl, err)
				continue
			}
			g.AddLinuxSysctl(split[0], split[1])
			continue
		}
		log.Warnf(ctx, "sysctl %q not of the format sysctl_name=value", defaultSysctl)
	}
	// extract linux sysctls from annotations and pass down to oci runtime
	// Will override any duplicate default systcl from crio.conf
	for key, value := range req.GetConfig().GetLinux().GetSysctls() {
		g.AddLinuxSysctl(key, value)
	}

	// Set OOM score adjust of the infra container to be very low
	// so it doesn't get killed.
	g.SetProcessOOMScoreAdj(PodInfraOOMAdj)

	g.SetLinuxResourcesCPUShares(PodInfraCPUshares)

	// set up namespaces
	if hostNetwork {
		err = g.RemoveLinuxNamespace(string(runtimespec.NetworkNamespace))
		if err != nil {
			return nil, err
		}
	} else if s.config.ManageNetworkNSLifecycle {
		// Create the sandbox network namespace
		if err := sb.NetNsCreate(nil); err != nil {
			return nil, err
		}

		defer func() {
			if err == nil {
				return
			}

			if netnsErr := sb.NetNsRemove(); netnsErr != nil {
				log.Warnf(ctx, "Failed to remove networking namespace: %v", netnsErr)
			}
		}()

		// Pass the created namespace path to the runtime
		err = g.AddOrReplaceLinuxNamespace(string(runtimespec.NetworkNamespace), sb.NetNsPath())
		if err != nil {
			return nil, err
		}
	}

	if securityContext.GetNamespaceOptions().GetPid() == pb.NamespaceMode_NODE {
		err = g.RemoveLinuxNamespace(string(runtimespec.PIDNamespace))
		if err != nil {
			return nil, err
		}
	}

	if securityContext.GetNamespaceOptions().GetIpc() == pb.NamespaceMode_NODE {
		err = g.RemoveLinuxNamespace(string(runtimespec.IPCNamespace))
		if err != nil {
			return nil, err
		}
	}

	if !s.seccompEnabled {
		g.Config.Linux.Seccomp = nil
	}

	saveOptions := generate.ExportOptions{}
	mountPoint, err := s.StorageRuntimeServer().StartContainer(id)
	if err != nil {
		return nil, fmt.Errorf("failed to mount container %s in pod sandbox %s(%s): %v", containerName, sb.Name(), id, err)
	}
	g.AddAnnotation(annotations.MountPoint, mountPoint)

	hostnamePath := fmt.Sprintf("%s/hostname", podContainer.RunDir)
	if err := ioutil.WriteFile(hostnamePath, []byte(hostname+"\n"), 0644); err != nil {
		return nil, err
	}
	if err := label.Relabel(hostnamePath, mountLabel, false); err != nil && err != unix.ENOTSUP {
		return nil, err
	}
	mnt = runtimespec.Mount{
		Type:        "bind",
		Source:      hostnamePath,
		Destination: "/etc/hostname",
		Options:     []string{"ro", "bind", "nodev", "nosuid", "noexec"},
	}
	pathsToChown = append(pathsToChown, hostnamePath)
	g.AddMount(mnt)
	g.AddAnnotation(annotations.HostnamePath, hostnamePath)
	sb.AddHostnamePath(hostnamePath)

	container, err := oci.NewContainer(id, containerName, podContainer.RunDir, logPath, sb.NetNs().Path(), labels, g.Config.Annotations, kubeAnnotations, "", "", "", nil, id, false, false, false, sb.Privileged(), sb.RuntimeHandler(), podContainer.Dir, created, podContainer.Config.Config.StopSignal)
	if err != nil {
		return nil, err
	}
	container.SetMountPoint(mountPoint)

	container.SetIDMappings(s.defaultIDMappings)

	if s.defaultIDMappings != nil && !s.defaultIDMappings.Empty() {
		if securityContext.GetNamespaceOptions().GetIpc() == pb.NamespaceMode_NODE {
			g.RemoveMount("/dev/mqueue")
			mqueue := runtimespec.Mount{
				Type:        "bind",
				Source:      "/dev/mqueue",
				Destination: "/dev/mqueue",
				Options:     []string{"rw", "rbind", "nodev", "nosuid", "noexec"},
			}
			g.AddMount(mqueue)
		}
		if hostNetwork {
			g.RemoveMount("/sys")
			g.RemoveMount("/sys/cgroup")
			sysMnt := spec.Mount{
				Destination: "/sys",
				Type:        "bind",
				Source:      "/sys",
				Options:     []string{"nosuid", "noexec", "nodev", "ro", "rbind"},
			}
			g.AddMount(sysMnt)
		}
		if securityContext.GetNamespaceOptions().GetPid() == pb.NamespaceMode_NODE {
			g.RemoveMount("/proc")
			proc := runtimespec.Mount{
				Type:        "bind",
				Source:      "/proc",
				Destination: "/proc",
				Options:     []string{"rw", "rbind", "nodev", "nosuid", "noexec"},
			}
			g.AddMount(proc)
		}
	}
	g.SetRootPath(mountPoint)

	if os.Getenv("_CRIO_ROOTLESS") != "" {
		makeOCIConfigurationRootless(&g)
	}

	container.SetSpec(g.Config)

	if err := sb.SetInfraContainer(container); err != nil {
		return nil, err
	}

	var ips []string
	var result cnitypes.Result

	if s.config.ManageNetworkNSLifecycle {
		ips, result, err = s.networkStart(ctx, sb)
		if err != nil {
			return nil, err
		}
		if result != nil {
			g.AddAnnotation(annotations.CNIResult, result.String())
		}
		defer func() {
			if err != nil {
				s.networkStop(ctx, sb)
			}
		}()
	}

	for idx, ip := range ips {
		g.AddAnnotation(fmt.Sprintf("%s.%d", annotations.IP, idx), ip)
	}
	sb.AddIPs(ips)
	sb.SetNamespaceOptions(securityContext.GetNamespaceOptions())

	spp := securityContext.GetSeccompProfilePath()
	g.AddAnnotation(annotations.SeccompProfilePath, spp)
	sb.SetSeccompProfilePath(spp)
	if !privileged {
		if err := s.setupSeccomp(ctx, &g, spp); err != nil {
			return nil, err
		}
	}

	err = g.SaveToFile(filepath.Join(podContainer.Dir, "config.json"), saveOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to save template configuration for pod sandbox %s(%s): %v", sb.Name(), id, err)
	}
	if err = g.SaveToFile(filepath.Join(podContainer.RunDir, "config.json"), saveOptions); err != nil {
		return nil, fmt.Errorf("failed to write runtime configuration for pod sandbox %s(%s): %v", sb.Name(), id, err)
	}

	s.addInfraContainer(container)
	defer func() {
		if err != nil {
			s.removeInfraContainer(container)
		}
	}()

	if s.defaultIDMappings != nil && !s.defaultIDMappings.Empty() {
		rootPair := s.defaultIDMappings.RootPair()
		for _, path := range pathsToChown {
			if err := os.Chown(path, rootPair.UID, rootPair.GID); err != nil {
				return nil, errors.Wrapf(err, "cannot chown %s to %d:%d", path, rootPair.UID, rootPair.GID)
			}
		}
	}

	if err := s.createContainerPlatform(container, nil, sb.CgroupParent()); err != nil {
		return nil, err
	}

	if err := s.Runtime().StartContainer(container); err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			// Clean-up steps from RemovePodSanbox
			timeout := int64(10)
			if err2 := s.Runtime().StopContainer(ctx, container, timeout); err2 != nil {
				log.Warnf(ctx, "failed to stop container %s: %v", container.Name(), err2)
			}
			if err2 := s.Runtime().WaitContainerStateStopped(ctx, container); err2 != nil {
				log.Warnf(ctx, "failed to get container 'stopped' status %s in pod sandbox %s: %v", container.Name(), sb.ID(), err2)
			}
			if err2 := s.Runtime().DeleteContainer(container); err2 != nil {
				log.Warnf(ctx, "failed to delete container %s in pod sandbox %s: %v", container.Name(), sb.ID(), err2)
			}
			if err2 := s.ContainerStateToDisk(container); err2 != nil {
				log.Warnf(ctx, "failed to write container state %s in pod sandbox %s: %v", container.Name(), sb.ID(), err2)
			}
		}
	}()

	if err := s.ContainerStateToDisk(container); err != nil {
		log.Warnf(ctx, "unable to write containers %s state to disk: %v", container.ID(), err)
	}

	if !s.config.ManageNetworkNSLifecycle {
		ips, _, err = s.networkStart(ctx, sb)
		if err != nil {
			return nil, err
		}
		defer func() {
			if err != nil {
				s.networkStop(ctx, sb)
			}
		}()
	}
	sb.AddIPs(ips)

	sb.SetCreated()

	log.Infof(ctx, "ran pod sandbox with infra container: %s", container.Description())
	resp = &pb.RunPodSandboxResponse{PodSandboxId: id}
	return resp, nil
}

func setupShm(podSandboxRunDir, mountLabel string) (shmPath string, err error) {
	shmPath = filepath.Join(podSandboxRunDir, "shm")
	if err := os.Mkdir(shmPath, 0700); err != nil {
		return "", err
	}
	shmOptions := "mode=1777,size=" + strconv.Itoa(sandbox.DefaultShmSize)
	if err = unix.Mount("shm", shmPath, "tmpfs", unix.MS_NOEXEC|unix.MS_NOSUID|unix.MS_NODEV,
		label.FormatMountLabel(shmOptions, mountLabel)); err != nil {
		return "", fmt.Errorf("failed to mount shm tmpfs for pod: %v", err)
	}
	return shmPath, nil
}

func AddCgroupAnnotation(ctx context.Context, g generate.Generator, mountPath, cgroupManager, cgroupParent, id string) (string, error) {
	if cgroupParent != "" {
		if cgroupManager == oci.SystemdCgroupsManager {
			if len(cgroupParent) <= 6 || !strings.HasSuffix(path.Base(cgroupParent), ".slice") {
				return "", fmt.Errorf("cri-o configured with systemd cgroup manager, but did not receive slice as parent: %s", cgroupParent)
			}
			cgPath := convertCgroupFsNameToSystemd(cgroupParent)
			g.SetLinuxCgroupsPath(cgPath + ":" + "crio" + ":" + id)
			cgroupParent = cgPath

			// check memory limit is greater than the minimum memory limit of 4Mb
			// expand the cgroup slice path
			slicePath, err := systemd.ExpandSlice(cgroupParent)
			if err != nil {
				return "", errors.Wrapf(err, "error expanding systemd slice path for %q", cgroupParent)
			}
			filename := ""
			cgroupv2, err := cgroups.IsCgroup2UnifiedMode()
			if err != nil {
				return "", err
			}
			if cgroupv2 {
				filename = "memory.max"
			} else {
				filename = "memory.limit_in_bytes"
			}

			// read in the memory limit from the memory.limit_in_bytes file
			fileData, err := ioutil.ReadFile(filepath.Join(mountPath, slicePath, filename))
			if err != nil {
				if os.IsNotExist(err) {
					log.Warnf(ctx, "Failed to find %s for slice: %q", filename, cgroupParent)
				} else {
					return "", errors.Wrapf(err, "error reading %s file for slice %q", filename, cgroupParent)
				}
			} else {
				// strip off the newline character and convert it to an int
				strMemory := strings.TrimRight(string(fileData), "\n")
				if strMemory != "" {
					memoryLimit, err := strconv.ParseInt(strMemory, 10, 64)
					if err != nil {
						return "", errors.Wrapf(err, "error converting cgroup memory value from string to int %q", strMemory)
					}
					// Compare with the minimum allowed memory limit
					if memoryLimit != 0 && memoryLimit < minMemoryLimit {
						return "", fmt.Errorf("pod set memory limit %v too low; should be at least %v", memoryLimit, minMemoryLimit)
					}
				}
			}
		} else {
			if strings.HasSuffix(path.Base(cgroupParent), ".slice") {
				return "", fmt.Errorf("cri-o configured with cgroupfs cgroup manager, but received systemd slice as parent: %s", cgroupParent)
			}
			cgPath := filepath.Join(cgroupParent, scopePrefix+"-"+id)
			g.SetLinuxCgroupsPath(cgPath)
		}
	}
	g.AddAnnotation(annotations.CgroupParent, cgroupParent)

	return cgroupParent, nil
}

// PauseCommand returns the pause command for the provided image configuration.
func PauseCommand(cfg *config.Config, image *v1.Image) ([]string, error) {
	if cfg == nil {
		return nil, fmt.Errorf("provided configuration is nil")
	}

	// This has been explicitly set by the user, since the configuration
	// default is `/pause`
	if cfg.PauseCommand == "" {
		if image == nil ||
			(len(image.Config.Entrypoint) == 0 && len(image.Config.Cmd) == 0) {
			return nil, fmt.Errorf(
				"unable to run pause image %q: %s",
				cfg.PauseImage,
				"neither Cmd nor Entrypoint specified",
			)
		}
		cmd := []string{}
		cmd = append(cmd, image.Config.Entrypoint...)
		cmd = append(cmd, image.Config.Cmd...)
		return cmd, nil
	}
	return []string{cfg.PauseCommand}, nil
}
