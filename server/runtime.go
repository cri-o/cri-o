package server

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/ocid/oci"
	"github.com/kubernetes-incubator/ocid/utils"
	pb "github.com/kubernetes/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	"github.com/opencontainers/ocitools/generate"
	"golang.org/x/net/context"
)

// Version returns the runtime name, runtime version and runtime API version
func (s *Server) Version(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
	version, err := getGPRCVersion()
	if err != nil {
		return nil, err
	}

	runtimeVersion, err := s.runtime.Version()
	if err != nil {
		return nil, err
	}

	// taking const address
	rav := runtimeAPIVersion
	runtimeName := s.runtime.Name()

	return &pb.VersionResponse{
		Version:           &version,
		RuntimeName:       &runtimeName,
		RuntimeVersion:    &runtimeVersion,
		RuntimeApiVersion: &rav,
	}, nil
}

// CreatePodSandbox creates a pod-level sandbox.
// The definition of PodSandbox is at https://github.com/kubernetes/kubernetes/pull/25899
func (s *Server) CreatePodSandbox(ctx context.Context, req *pb.CreatePodSandboxRequest) (*pb.CreatePodSandboxResponse, error) {
	var err error

	if err := os.MkdirAll(s.sandboxDir, 0755); err != nil {
		return nil, err
	}

	// process req.Name
	name := req.GetConfig().GetMetadata().GetName()
	if name == "" {
		return nil, fmt.Errorf("PodSandboxConfig.Name should not be empty")
	}

	podSandboxDir := filepath.Join(s.sandboxDir, name)
	if _, err := os.Stat(podSandboxDir); err == nil {
		return nil, fmt.Errorf("pod sandbox (%s) already exists", podSandboxDir)
	}

	if err := os.MkdirAll(podSandboxDir, 0755); err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			os.RemoveAll(podSandboxDir)
		}
	}()

	// creates a spec Generator with the default spec.
	g := generate.New()

	// setup defaults for the pod sandbox
	g.SetRootPath("/var/lib/ocid/graph/vfs/pause")
	g.SetRootReadonly(true)
	g.SetProcessArgs([]string{"/pause"})

	// process req.Hostname
	hostname := req.GetConfig().GetHostname()
	if hostname != "" {
		g.SetHostname(hostname)
	}

	// process req.LogDirectory
	logDir := req.GetConfig().GetLogDirectory()
	if logDir == "" {
		logDir = fmt.Sprintf("/var/log/ocid/pods/%s", name)
	}

	dnsServers := req.GetConfig().GetDnsOptions().GetServers()
	dnsSearches := req.GetConfig().GetDnsOptions().GetSearches()
	resolvPath := fmt.Sprintf("%s/resolv.conf", podSandboxDir)
	err = parseDNSOptions(dnsServers, dnsSearches, resolvPath)
	if err != nil {
		err1 := removeFile(resolvPath)
		if err1 != nil {
			err = err1
			return nil, fmt.Errorf("%v; failed to remove %s: %v", err, resolvPath, err1)
		}
		return nil, err
	}

	g.AddBindMount(resolvPath, "/etc/resolv.conf", "ro")

	labels := req.GetConfig().GetLabels()
	s.addSandbox(&sandbox{
		name:       name,
		logDir:     logDir,
		labels:     labels,
		containers: make(map[string]*oci.Container),
	})

	annotations := req.GetConfig().GetAnnotations()
	for k, v := range annotations {
		g.AddAnnotation(k, v)
	}

	cgroupParent := req.GetConfig().GetLinux().GetCgroupParent()
	if cgroupParent != "" {
		g.SetLinuxCgroupsPath(cgroupParent)
	}

	// set up namespaces
	if req.GetConfig().GetLinux().GetNamespaceOptions().GetHostNetwork() {
		err = g.RemoveLinuxNamespace("network")
		if err != nil {
			return nil, err
		}
	}

	if req.GetConfig().GetLinux().GetNamespaceOptions().GetHostPid() {
		err = g.RemoveLinuxNamespace("pid")
		if err != nil {
			return nil, err
		}
	}

	if req.GetConfig().GetLinux().GetNamespaceOptions().GetHostIpc() {
		err = g.RemoveLinuxNamespace("ipc")
		if err != nil {
			return nil, err
		}
	}

	err = g.SaveToFile(filepath.Join(podSandboxDir, "config.json"))
	if err != nil {
		return nil, err
	}

	containerName := name + "-infra"
	container, err := oci.NewContainer(containerName, podSandboxDir, podSandboxDir, labels, name, false)
	if err != nil {
		return nil, err
	}

	if err = s.runtime.CreateContainer(container); err != nil {
		return nil, err
	}

	if err = s.runtime.UpdateStatus(container); err != nil {
		return nil, err
	}

	// Setup the network
	podNamespace := ""
	netnsPath, err := container.NetNsPath()
	if err != nil {
		return nil, err
	}
	if err = s.netPlugin.SetUpPod(netnsPath, podNamespace, name, containerName); err != nil {
		return nil, fmt.Errorf("failed to create network for container %s in sandbox %s: %v", containerName, name, err)
	}

	if err = s.runtime.StartContainer(container); err != nil {
		return nil, err
	}

	s.addContainer(container)

	if err = s.runtime.UpdateStatus(container); err != nil {
		return nil, err
	}

	return &pb.CreatePodSandboxResponse{PodSandboxId: &name}, nil
}

// StopPodSandbox stops the sandbox. If there are any running containers in the
// sandbox, they should be force terminated.
func (s *Server) StopPodSandbox(ctx context.Context, req *pb.StopPodSandboxRequest) (*pb.StopPodSandboxResponse, error) {
	sbName := req.PodSandboxId
	if *sbName == "" {
		return nil, fmt.Errorf("PodSandboxId should not be empty")
	}
	sb := s.getSandbox(*sbName)
	if sb == nil {
		return nil, fmt.Errorf("specified sandbox not found: %s", *sbName)
	}

	podInfraContainer := *sbName + "-infra"
	sb.containersLock.Lock()
	for _, c := range sb.containers {
		if podInfraContainer == c.Name() {
			podNamespace := ""
			netnsPath, err := c.NetNsPath()
			if err != nil {
				sb.containersLock.Unlock()
				return nil, err
			}
			if err := s.netPlugin.TearDownPod(netnsPath, podNamespace, *sbName, podInfraContainer); err != nil {
				sb.containersLock.Unlock()
				return nil, fmt.Errorf("failed to destroy network for container %s in sandbox %s: %v", c.Name(), *sbName, err)
			}
		}
		if err := s.runtime.StopContainer(c); err != nil {
			sb.containersLock.Unlock()
			return nil, fmt.Errorf("failed to stop container %s in sandbox %s: %v", c.Name(), *sbName, err)
		}
	}
	sb.containersLock.Unlock()

	return &pb.StopPodSandboxResponse{}, nil
}

// RemovePodSandbox deletes the sandbox. If there are any running containers in the
// sandbox, they should be force deleted.
func (s *Server) RemovePodSandbox(ctx context.Context, req *pb.RemovePodSandboxRequest) (*pb.RemovePodSandboxResponse, error) {
	sbName := req.PodSandboxId
	if *sbName == "" {
		return nil, fmt.Errorf("PodSandboxId should not be empty")
	}
	sb := s.getSandbox(*sbName)
	if sb == nil {
		return nil, fmt.Errorf("specified sandbox not found: %s", *sbName)
	}

	podInfraContainer := *sbName + "-infra"

	// Delete all the containers in the sandbox
	sb.containersLock.Lock()
	for _, c := range sb.containers {
		if err := s.runtime.DeleteContainer(c); err != nil {
			sb.containersLock.Unlock()
			return nil, fmt.Errorf("failed to delete container %s in sandbox %s: %v", c.Name(), *sbName, err)
		}
		if podInfraContainer == c.Name() {
			continue
		}
		containerDir := filepath.Join(s.runtime.ContainerDir(), c.Name())
		if err := os.RemoveAll(containerDir); err != nil {
			sb.containersLock.Unlock()
			return nil, fmt.Errorf("failed to remove container %s directory: %v", c.Name(), err)
		}
	}
	sb.containersLock.Unlock()

	// Remove the files related to the sandbox
	podSandboxDir := filepath.Join(s.sandboxDir, *sbName)
	if err := os.RemoveAll(podSandboxDir); err != nil {
		return nil, fmt.Errorf("failed to remove sandbox %s directory: %v", *sbName, err)
	}

	return &pb.RemovePodSandboxResponse{}, nil
}

func int64Ptr(i int64) *int64 {
	return &i
}

func int32Ptr(i int32) *int32 {
	return &i
}

func sPtr(s string) *string {
	return &s
}

// PodSandboxStatus returns the Status of the PodSandbox.
func (s *Server) PodSandboxStatus(ctx context.Context, req *pb.PodSandboxStatusRequest) (*pb.PodSandboxStatusResponse, error) {
	sbName := req.PodSandboxId
	if *sbName == "" {
		return nil, fmt.Errorf("PodSandboxId should not be empty")
	}
	sb := s.getSandbox(*sbName)
	if sb == nil {
		return nil, fmt.Errorf("specified sandbox not found: %s", *sbName)
	}

	podInfraContainerName := *sbName + "-infra"
	podInfraContainer := sb.getContainer(podInfraContainerName)

	cState := s.runtime.ContainerStatus(podInfraContainer)
	created := cState.Created.Unix()

	netNsPath, err := podInfraContainer.NetNsPath()
	if err != nil {
		return nil, err
	}
	podNamespace := ""
	ip, err := s.netPlugin.GetContainerNetworkStatus(netNsPath, podNamespace, *sbName, podInfraContainerName)
	if err != nil {
		// ignore the error on network status
		ip = ""
	}

	return &pb.PodSandboxStatusResponse{
		Status: &pb.PodSandboxStatus{
			Id:        sbName,
			CreatedAt: int64Ptr(created),
			Linux: &pb.LinuxPodSandboxStatus{
				Namespaces: &pb.Namespace{
					Network: sPtr(netNsPath),
				},
			},
			Network: &pb.PodSandboxNetworkStatus{Ip: &ip},
		},
	}, nil
}

// ListPodSandbox returns a list of SandBox.
func (s *Server) ListPodSandbox(context.Context, *pb.ListPodSandboxRequest) (*pb.ListPodSandboxResponse, error) {
	return nil, nil
}

// CreateContainer creates a new container in specified PodSandbox
func (s *Server) CreateContainer(ctx context.Context, req *pb.CreateContainerRequest) (*pb.CreateContainerResponse, error) {
	// The id of the PodSandbox
	podSandboxId := req.GetPodSandboxId()
	if !s.hasSandbox(podSandboxId) {
		return nil, fmt.Errorf("the pod sandbox (%s) does not exist", podSandboxId)
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

	if _, err := os.Stat(containerDir); err == nil {
		return nil, fmt.Errorf("container (%s) already exists", containerDir)
	}

	if err := os.MkdirAll(containerDir, 0755); err != nil {
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
		selinuxRelabel := mount.GetSelinuxRelabel()
		fmt.Printf("selinuxRelabel: %v\n", selinuxRelabel)

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

	// The config of the PodSandbox
	sandboxConfig := req.GetSandboxConfig()
	fmt.Printf("sandboxConfig: %v\n", sandboxConfig)

	// Join the namespace paths for the pod sandbox container.
	podContainerName := podSandboxId + "-infra"
	podInfraContainer := s.state.containers[podContainerName]
	podInfraState := s.runtime.ContainerStatus(podInfraContainer)

	logrus.Infof("pod container state %v", podInfraState)

	for nsType, nsFile := range map[string]string{
		"ipc":     "ipc",
		"uts":     "uts",
		"network": "net",
	} {
		nsPath := fmt.Sprintf("/proc/%d/ns/%s", podInfraState.Pid, nsFile)
		specgen.AddOrReplaceLinuxNamespace(nsType, nsPath)
	}

	if err := specgen.SaveToFile(filepath.Join(containerDir, "config.json")); err != nil {
		return nil, err
	}

	// TODO: copy the rootfs into the bundle.
	// Currently, utils.CreateFakeRootfs is used to populate the rootfs.
	if err := utils.CreateFakeRootfs(containerDir, image); err != nil {
		return nil, err
	}

	container, err := oci.NewContainer(name, containerDir, logPath, labels, podSandboxId, containerConfig.GetTty())
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

// StartContainer starts the container.
func (s *Server) StartContainer(ctx context.Context, req *pb.StartContainerRequest) (*pb.StartContainerResponse, error) {
	containerName := req.ContainerId

	if *containerName == "" {
		return nil, fmt.Errorf("container ID should not be empty")
	}
	c := s.state.containers[*containerName]
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
	c := s.state.containers[*containerName]
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
	c := s.state.containers[*containerName]
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
func (s *Server) ListContainers(context.Context, *pb.ListContainersRequest) (*pb.ListContainersResponse, error) {
	return nil, nil
}

// ContainerStatus returns status of the container.
func (s *Server) ContainerStatus(ctx context.Context, req *pb.ContainerStatusRequest) (*pb.ContainerStatusResponse, error) {
	containerName := req.ContainerId

	if *containerName == "" {
		return nil, fmt.Errorf("container ID should not be empty")
	}
	c := s.state.containers[*containerName]

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
	case "created":
		rStatus = pb.ContainerState_CREATED
		created := cState.Created.Unix()
		csr.Status.CreatedAt = int64Ptr(created)
	case "running":
		rStatus = pb.ContainerState_RUNNING
		created := cState.Created.Unix()
		csr.Status.CreatedAt = int64Ptr(created)
		started := cState.Started.Unix()
		csr.Status.StartedAt = int64Ptr(started)
	case "stopped":
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
