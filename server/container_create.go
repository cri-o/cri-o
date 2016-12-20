package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/stringid"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/server/apparmor"
	"github.com/kubernetes-incubator/cri-o/server/seccomp"
	"github.com/kubernetes-incubator/cri-o/utils"
	"github.com/opencontainers/runc/libcontainer/label"
	"github.com/opencontainers/runtime-tools/generate"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

const (
	seccompUnconfined      = "unconfined"
	seccompRuntimeDefault  = "runtime/default"
	seccompLocalhostPrefix = "localhost/"
)

// CreateContainer creates a new container in specified PodSandbox
func (s *Server) CreateContainer(ctx context.Context, req *pb.CreateContainerRequest) (res *pb.CreateContainerResponse, err error) {
	logrus.Debugf("CreateContainerRequest %+v", req)
	sbID := req.GetPodSandboxId()
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

	name := containerConfig.GetMetadata().GetName()
	if name == "" {
		return nil, fmt.Errorf("CreateContainerRequest.ContainerConfig.Name is empty")
	}

	attempt := containerConfig.GetMetadata().GetAttempt()
	containerID, containerName, err := s.generateContainerIDandName(sb.name, name, attempt)
	if err != nil {
		return nil, err
	}

	// containerDir is the dir for the container bundle.
	containerDir := filepath.Join(s.runtime.ContainerDir(), containerID)
	defer func() {
		if err != nil {
			s.releaseContainerName(containerName)
			err1 := os.RemoveAll(containerDir)
			if err1 != nil {
				logrus.Warnf("Failed to cleanup container directory: %v", err1)
			}
		}
	}()

	if _, err = os.Stat(containerDir); err == nil {
		return nil, fmt.Errorf("container (%s) already exists", containerDir)
	}

	if err = os.MkdirAll(containerDir, 0755); err != nil {
		return nil, err
	}

	container, err := s.createSandboxContainer(containerID, containerName, sb, containerDir, containerConfig)
	if err != nil {
		return nil, err
	}

	if err = s.runtime.CreateContainer(container); err != nil {
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

	resp := &pb.CreateContainerResponse{
		ContainerId: &containerID,
	}

	logrus.Debugf("CreateContainerResponse: %+v", resp)
	return resp, nil
}

func (s *Server) createSandboxContainer(containerID string, containerName string, sb *sandbox, containerDir string, containerConfig *pb.ContainerConfig) (*oci.Container, error) {
	if sb == nil {
		return nil, errors.New("createSandboxContainer needs a sandbox")
	}
	// creates a spec Generator with the default spec.
	specgen := generate.New()

	// by default, the root path is an empty string.
	// here set it to be "rootfs".
	specgen.SetRootPath("rootfs")

	processArgs := []string{}
	commands := containerConfig.GetCommand()
	args := containerConfig.GetArgs()
	if commands == nil && args == nil {
		// TODO: override with image's config in #189
		processArgs = []string{"/bin/sh"}
	}
	if commands != nil {
		processArgs = append(processArgs, commands...)
	}
	if args != nil {
		processArgs = append(processArgs, args...)
	}

	specgen.SetProcessArgs(processArgs)

	cwd := containerConfig.GetWorkingDir()
	if cwd == "" {
		cwd = "/"
	}
	specgen.SetProcessCwd(cwd)

	envs := containerConfig.GetEnvs()
	if envs != nil {
		for _, item := range envs {
			key := item.GetKey()
			value := item.GetValue()
			if key == "" {
				continue
			}
			env := fmt.Sprintf("%s=%s", key, value)
			specgen.AddProcessEnv(env)
		}
	}

	mounts := containerConfig.GetMounts()
	for _, mount := range mounts {
		dest := mount.GetContainerPath()
		if dest == "" {
			return nil, fmt.Errorf("Mount.ContainerPath is empty")
		}

		src := mount.GetHostPath()
		if src == "" {
			return nil, fmt.Errorf("Mount.HostPath is empty")
		}

		options := []string{"rw"}
		if mount.GetReadonly() {
			options = []string{"ro"}
		}

		if mount.GetSelinuxRelabel() {
			// Need a way in kubernetes to determine if the volume is shared or private
			if err := label.Relabel(src, sb.mountLabel, true); err != nil && err != syscall.ENOTSUP {
				return nil, fmt.Errorf("relabel failed %s: %v", src, err)
			}
		}

		specgen.AddBindMount(src, dest, options)
	}

	labels := containerConfig.GetLabels()

	metadata := containerConfig.GetMetadata()

	annotations := containerConfig.GetAnnotations()
	if annotations != nil {
		for k, v := range annotations {
			specgen.AddAnnotation(k, v)
		}
	}

	// set this container's apparmor profile if it is set by sandbox
	if s.appArmorEnabled {
		appArmorProfileName := s.getAppArmorProfileName(sb.annotations, metadata.GetName())
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

	if containerConfig.GetLinux().GetSecurityContext().GetPrivileged() {
		specgen.SetupPrivileged(true)
	}

	if containerConfig.GetLinux().GetSecurityContext().GetReadonlyRootfs() {
		specgen.SetRootReadonly(true)
	}

	logPath := containerConfig.GetLogPath()

	if containerConfig.GetTty() {
		specgen.SetProcessTerminal(true)
	}

	linux := containerConfig.GetLinux()
	if linux != nil {
		resources := linux.GetResources()
		if resources != nil {
			cpuPeriod := resources.GetCpuPeriod()
			if cpuPeriod != 0 {
				specgen.SetLinuxResourcesCPUPeriod(uint64(cpuPeriod))
			}

			cpuQuota := resources.GetCpuQuota()
			if cpuQuota != 0 {
				specgen.SetLinuxResourcesCPUQuota(uint64(cpuQuota))
			}

			cpuShares := resources.GetCpuShares()
			if cpuShares != 0 {
				specgen.SetLinuxResourcesCPUShares(uint64(cpuShares))
			}

			memoryLimit := resources.GetMemoryLimitInBytes()
			if memoryLimit != 0 {
				specgen.SetLinuxResourcesMemoryLimit(uint64(memoryLimit))
			}

			oomScoreAdj := resources.GetOomScoreAdj()
			specgen.SetLinuxResourcesOOMScoreAdj(int(oomScoreAdj))
		}

		if sb.cgroupParent != "" {
			if s.config.CgroupManager == "systemd" {
				cgPath := sb.cgroupParent + ":" + "ocid" + ":" + containerID
				specgen.SetLinuxCgroupsPath(cgPath)
			} else {
				specgen.SetLinuxCgroupsPath(sb.cgroupParent + "/" + containerID)
			}
		}

		capabilities := linux.GetSecurityContext().GetCapabilities()
		if capabilities != nil {
			addCaps := capabilities.GetAddCapabilities()
			if addCaps != nil {
				for _, cap := range addCaps {
					if err := specgen.AddProcessCapability(cap); err != nil {
						return nil, err
					}
				}
			}

			dropCaps := capabilities.GetDropCapabilities()
			if dropCaps != nil {
				for _, cap := range dropCaps {
					if err := specgen.DropProcessCapability(cap); err != nil {
						return nil, err
					}
				}
			}
		}

		specgen.SetProcessSelinuxLabel(sb.processLabel)
		specgen.SetLinuxMountLabel(sb.mountLabel)

		user := linux.GetSecurityContext().GetRunAsUser()
		specgen.SetProcessUID(uint32(user))

		specgen.SetProcessGID(uint32(user))

		groups := linux.GetSecurityContext().GetSupplementalGroups()
		for _, group := range groups {
			specgen.AddProcessAdditionalGid(uint32(group))
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

	image := imageSpec.GetImage()
	if image == "" {
		return nil, fmt.Errorf("CreateContainerRequest.ContainerConfig.Image.Image is empty")
	}

	// bind mount the pod shm
	specgen.AddBindMount(sb.shmPath, "/dev/shm", []string{"rw"})

	specgen.AddAnnotation("ocid/name", containerName)
	specgen.AddAnnotation("ocid/sandbox_id", sb.id)
	specgen.AddAnnotation("ocid/sandbox_name", sb.infraContainer.Name())
	specgen.AddAnnotation("ocid/container_type", containerTypeContainer)
	specgen.AddAnnotation("ocid/log_path", logPath)
	specgen.AddAnnotation("ocid/tty", fmt.Sprintf("%v", containerConfig.GetTty()))
	specgen.AddAnnotation("ocid/image", image)

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	specgen.AddAnnotation("ocid/metadata", string(metadataJSON))

	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, err
	}
	specgen.AddAnnotation("ocid/labels", string(labelsJSON))

	annotationsJSON, err := json.Marshal(annotations)
	if err != nil {
		return nil, err
	}
	specgen.AddAnnotation("ocid/annotations", string(annotationsJSON))

	if err = s.setupSeccomp(&specgen, containerName, sb.annotations); err != nil {
		return nil, err
	}

	if err = specgen.SaveToFile(filepath.Join(containerDir, "config.json"), generate.ExportOptions{}); err != nil {
		return nil, err
	}

	// TODO: copy the rootfs into the bundle.
	// Currently, utils.CreateFakeRootfs is used to populate the rootfs.
	if err = utils.CreateFakeRootfs(containerDir, image); err != nil {
		return nil, err
	}

	container, err := oci.NewContainer(containerID, containerName, containerDir, logPath, sb.netNs(), labels, annotations, imageSpec, metadata, sb.id, containerConfig.GetTty())
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
