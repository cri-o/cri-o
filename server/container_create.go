package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/stringid"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/utils"
	"github.com/opencontainers/runc/libcontainer/label"
	"github.com/opencontainers/runtime-tools/generate"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
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
				logrus.Warnf("Failed to cleanup container directory: %v")
			}
		}
	}()

	if _, err = os.Stat(containerDir); err == nil {
		return nil, fmt.Errorf("container (%s) already exists", containerDir)
	}

	if err = os.MkdirAll(containerDir, 0755); err != nil {
		return nil, err
	}

	container, err := s.createSandboxContainer(containerID, containerName, sb, req.GetSandboxConfig(), containerDir, containerConfig)
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

func (s *Server) createSandboxContainer(containerID string, containerName string, sb *sandbox, SandboxConfig *pb.PodSandboxConfig, containerDir string, containerConfig *pb.ContainerConfig) (*oci.Container, error) {
	if sb == nil {
		return nil, errors.New("createSandboxContainer needs a sandbox")
	}
	// creates a spec Generator with the default spec.
	specgen := generate.New()

	// by default, the root path is an empty string.
	// here set it to be "rootfs".
	specgen.SetRootPath("rootfs")

	args := containerConfig.GetArgs()
	if args == nil {
		args = []string{"/bin/sh"}
	}
	specgen.SetProcessArgs(args)

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

	for nsType, nsFile := range map[string]string{
		"ipc":     "ipc",
		"network": "net",
	} {
		nsPath := fmt.Sprintf("/proc/%d/ns/%s", podInfraState.Pid, nsFile)
		if err := specgen.AddOrReplaceLinuxNamespace(nsType, nsPath); err != nil {
			return nil, err
		}
	}

	specgen.AddAnnotation("ocid/name", containerName)
	specgen.AddAnnotation("ocid/sandbox_id", sb.id)
	specgen.AddAnnotation("ocid/log_path", logPath)
	specgen.AddAnnotation("ocid/tty", fmt.Sprintf("%v", containerConfig.GetTty()))

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

	if err = specgen.SaveToFile(filepath.Join(containerDir, "config.json"), generate.ExportOptions{}); err != nil {
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

	// TODO: copy the rootfs into the bundle.
	// Currently, utils.CreateFakeRootfs is used to populate the rootfs.
	if err = utils.CreateFakeRootfs(containerDir, image); err != nil {
		return nil, err
	}

	container, err := oci.NewContainer(containerID, containerName, containerDir, logPath, labels, metadata, sb.id, containerConfig.GetTty())
	if err != nil {
		return nil, err
	}

	return container, nil
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
