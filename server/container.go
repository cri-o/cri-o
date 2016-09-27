package server

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/utils"
	"github.com/opencontainers/runtime-tools/generate"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

const (
	// ContainerStateCreated represents the created state of a container
	ContainerStateCreated = "created"
	// ContainerStateRunning represents the running state of a container
	ContainerStateRunning = "running"
	// ContainerStateStopped represents the stopped state of a container
	ContainerStateStopped = "stopped"
)

// CreateContainer creates a new container in specified PodSandbox
func (s *Server) CreateContainer(ctx context.Context, req *pb.CreateContainerRequest) (*pb.CreateContainerResponse, error) {
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

	// containerDir is the dir for the container bundle.
	containerDir := filepath.Join(s.runtime.ContainerDir(), name)

	if _, err = os.Stat(containerDir); err == nil {
		return nil, fmt.Errorf("container (%s) already exists", containerDir)
	}

	if err = os.MkdirAll(containerDir, 0755); err != nil {
		return nil, err
	}

	container, err := s.createSandboxContainer(name, sb, req.GetSandboxConfig(), containerDir, containerConfig)
	if err != nil {
		return nil, err
	}

	if err := s.runtime.CreateContainer(container); err != nil {
		return nil, err
	}

	if err := s.runtime.UpdateStatus(container); err != nil {
		return nil, err
	}

	s.addContainer(container)

	return &pb.CreateContainerResponse{
		ContainerId: &name,
	}, nil
}

func (s *Server) createSandboxContainer(name string, sb *sandbox, SandboxConfig *pb.PodSandboxConfig, containerDir string, containerConfig *pb.ContainerConfig) (*oci.Container, error) {
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

		options := "rw"
		if mount.GetReadonly() {
			options = "ro"
		}

		//TODO(hmeng): how to use this info? Do we need to handle relabel a FS with Selinux?
		//selinuxRelabel := mount.GetSelinuxRelabel()

		specgen.AddBindMount(src, dest, options)

	}

	labels := containerConfig.GetLabels()

	annotations := containerConfig.GetAnnotations()
	if annotations != nil {
		for k, v := range annotations {
			specgen.AddAnnotation(k, v)
		}
	}

	if containerConfig.GetPrivileged() {
		specgen.SetupPrivileged(true)
	}

	if containerConfig.GetReadonlyRootfs() {
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

		capabilities := linux.GetCapabilities()
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

		selinuxOptions := linux.GetSelinuxOptions()
		if selinuxOptions != nil {
			user := selinuxOptions.GetUser()
			if user == "" {
				return nil, fmt.Errorf("SELinuxOption.User is empty")
			}

			role := selinuxOptions.GetRole()
			if role == "" {
				return nil, fmt.Errorf("SELinuxOption.Role is empty")
			}

			t := selinuxOptions.GetType()
			if t == "" {
				return nil, fmt.Errorf("SELinuxOption.Type is empty")
			}

			level := selinuxOptions.GetLevel()
			if level == "" {
				return nil, fmt.Errorf("SELinuxOption.Level is empty")
			}

			specgen.SetProcessSelinuxLabel(fmt.Sprintf("%s:%s:%s:%s", user, role, t, level))
		}

		user := linux.GetUser()
		if user != nil {
			uid := user.GetUid()
			specgen.SetProcessUID(uint32(uid))

			gid := user.GetGid()
			specgen.SetProcessGID(uint32(gid))

			groups := user.GetAdditionalGids()
			if groups != nil {
				for _, group := range groups {
					specgen.AddProcessAdditionalGid(uint32(group))
				}
			}
		}
	}
	// Join the namespace paths for the pod sandbox container.
	podContainerName := sb.name + "-infra"
	podInfraContainer := s.state.containers.Get(podContainerName)
	podInfraState := s.runtime.ContainerStatus(podInfraContainer)

	logrus.Infof("pod container state %v", podInfraState)

	for nsType, nsFile := range map[string]string{
		"ipc":     "ipc",
		"uts":     "uts",
		"network": "net",
	} {
		nsPath := fmt.Sprintf("/proc/%d/ns/%s", podInfraState.Pid, nsFile)
		if err := specgen.AddOrReplaceLinuxNamespace(nsType, nsPath); err != nil {
			return nil, err
		}
	}

	if err := specgen.SaveToFile(filepath.Join(containerDir, "config.json")); err != nil {
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
	if err := utils.CreateFakeRootfs(containerDir, image); err != nil {
		return nil, err
	}

	container, err := oci.NewContainer(name, containerDir, logPath, labels, sb.id, containerConfig.GetTty())
	if err != nil {
		return nil, err
	}

	return container, nil
}

// StartContainer starts the container.
func (s *Server) StartContainer(ctx context.Context, req *pb.StartContainerRequest) (*pb.StartContainerResponse, error) {
	containerName := req.ContainerId

	if *containerName == "" {
		return nil, fmt.Errorf("container ID should not be empty")
	}
	c := s.state.containers.Get(*containerName)
	if c == nil {
		return nil, fmt.Errorf("specified container not found: %s", *containerName)
	}

	if err := s.runtime.StartContainer(c); err != nil {
		return nil, fmt.Errorf("failed to start container %s in sandbox %s: %v", c.Name(), *containerName, err)
	}

	return &pb.StartContainerResponse{}, nil
}

// StopContainer stops a running container with a grace period (i.e., timeout).
func (s *Server) StopContainer(ctx context.Context, req *pb.StopContainerRequest) (*pb.StopContainerResponse, error) {
	containerName := req.ContainerId

	if *containerName == "" {
		return nil, fmt.Errorf("container ID should not be empty")
	}
	c := s.state.containers.Get(*containerName)
	if c == nil {
		return nil, fmt.Errorf("specified container not found: %s", *containerName)
	}

	if err := s.runtime.StopContainer(c); err != nil {
		return nil, fmt.Errorf("failed to stop container %s: %v", *containerName, err)
	}

	return &pb.StopContainerResponse{}, nil
}

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (s *Server) RemoveContainer(ctx context.Context, req *pb.RemoveContainerRequest) (*pb.RemoveContainerResponse, error) {
	containerName := req.ContainerId

	if *containerName == "" {
		return nil, fmt.Errorf("container ID should not be empty")
	}
	c := s.state.containers.Get(*containerName)
	if c == nil {
		return nil, fmt.Errorf("specified container not found: %s", *containerName)
	}

	if err := s.runtime.DeleteContainer(c); err != nil {
		return nil, fmt.Errorf("failed to delete container %s: %v", *containerName, err)
	}

	containerDir := filepath.Join(s.runtime.ContainerDir(), *containerName)
	if err := os.RemoveAll(containerDir); err != nil {
		return nil, fmt.Errorf("failed to remove container %s directory: %v", *containerName, err)
	}

	s.removeContainer(c)

	return &pb.RemoveContainerResponse{}, nil
}

// ListContainers lists all containers by filters.
func (s *Server) ListContainers(ctx context.Context, req *pb.ListContainersRequest) (*pb.ListContainersResponse, error) {
	var ctrs []*pb.Container
	for _, ctr := range s.state.containers.List() {
		if err := s.runtime.UpdateStatus(ctr); err != nil {
			return nil, err
		}

		podSandboxID := ctr.Sandbox()
		cState := s.runtime.ContainerStatus(ctr)
		created := cState.Created.Unix()
		rState := pb.ContainerState_UNKNOWN

		c := &pb.Container{
			Id:           &cState.ID,
			PodSandboxId: &podSandboxID,
			CreatedAt:    int64Ptr(created),
		}

		switch cState.Status {
		case ContainerStateCreated:
			rState = pb.ContainerState_CREATED
		case ContainerStateRunning:
			rState = pb.ContainerState_RUNNING
		case ContainerStateStopped:
			rState = pb.ContainerState_EXITED
		}
		c.State = &rState

		ctrs = append(ctrs, c)
	}

	return &pb.ListContainersResponse{
		Containers: ctrs,
	}, nil
}

// ContainerStatus returns status of the container.
func (s *Server) ContainerStatus(ctx context.Context, req *pb.ContainerStatusRequest) (*pb.ContainerStatusResponse, error) {
	containerName := req.ContainerId

	if *containerName == "" {
		return nil, fmt.Errorf("container ID should not be empty")
	}
	c := s.state.containers.Get(*containerName)

	if c == nil {
		return nil, fmt.Errorf("specified container not found: %s", *containerName)
	}

	if err := s.runtime.UpdateStatus(c); err != nil {
		return nil, err
	}

	csr := &pb.ContainerStatusResponse{
		Status: &pb.ContainerStatus{
			Id: containerName,
		},
	}

	cState := s.runtime.ContainerStatus(c)
	rStatus := pb.ContainerState_UNKNOWN

	switch cState.Status {
	case ContainerStateCreated:
		rStatus = pb.ContainerState_CREATED
		created := cState.Created.Unix()
		csr.Status.CreatedAt = int64Ptr(created)
	case ContainerStateRunning:
		rStatus = pb.ContainerState_RUNNING
		created := cState.Created.Unix()
		csr.Status.CreatedAt = int64Ptr(created)
		started := cState.Started.Unix()
		csr.Status.StartedAt = int64Ptr(started)
	case ContainerStateStopped:
		rStatus = pb.ContainerState_EXITED
		created := cState.Created.Unix()
		csr.Status.CreatedAt = int64Ptr(created)
		started := cState.Started.Unix()
		csr.Status.StartedAt = int64Ptr(started)
		finished := cState.Finished.Unix()
		csr.Status.FinishedAt = int64Ptr(finished)
		csr.Status.ExitCode = int32Ptr(cState.ExitCode)
	}

	csr.Status.State = &rStatus

	return csr, nil
}

// Exec executes the command in the container.
func (s *Server) Exec(pb.RuntimeService_ExecServer) error {
	return nil
}
