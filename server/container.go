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
	"k8s.io/kubernetes/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

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

type containerRequest interface {
	GetContainerId() string
}

func (s *Server) getContainerFromRequest(req containerRequest) (*oci.Container, error) {
	ctrID := req.GetContainerId()
	if ctrID == "" {
		return nil, fmt.Errorf("container ID should not be empty")
	}

	containerID, err := s.ctrIDIndex.Get(ctrID)
	if err != nil {
		return nil, fmt.Errorf("container with ID starting with %s not found: %v", ctrID, err)
	}

	c := s.state.containers.Get(containerID)
	if c == nil {
		return nil, fmt.Errorf("specified container not found: %s", containerID)
	}
	return c, nil
}

// CreateContainer creates a new container in specified PodSandbox
func (s *Server) CreateContainer(ctx context.Context, req *pb.CreateContainerRequest) (res *pb.CreateContainerResponse, err error) {
	logrus.Debugf("CreateContainer %+v", req)
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

	return &pb.CreateContainerResponse{
		ContainerId: &containerID,
	}, nil
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

		options := "rw"
		if mount.GetReadonly() {
			options = "ro"
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

		specgen.SetProcessSelinuxLabel(sb.processLabel)
		specgen.SetLinuxMountLabel(sb.mountLabel)

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
	podInfraState := s.runtime.ContainerStatus(sb.infraContainer)

	logrus.Infof("pod container state %v", podInfraState)

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
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, err
	}

	specgen.AddAnnotation("ocid/labels", string(labelsJSON))

	if err = specgen.SaveToFile(filepath.Join(containerDir, "config.json")); err != nil {
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

	container, err := oci.NewContainer(containerID, containerName, containerDir, logPath, labels, sb.id, containerConfig.GetTty())
	if err != nil {
		return nil, err
	}

	return container, nil
}

// StartContainer starts the container.
func (s *Server) StartContainer(ctx context.Context, req *pb.StartContainerRequest) (*pb.StartContainerResponse, error) {
	logrus.Debugf("StartContainer %+v", req)
	c, err := s.getContainerFromRequest(req)
	if err != nil {
		return nil, err
	}

	if err := s.runtime.StartContainer(c); err != nil {
		return nil, fmt.Errorf("failed to start container %s: %v", c.ID(), err)
	}

	return &pb.StartContainerResponse{}, nil
}

// StopContainer stops a running container with a grace period (i.e., timeout).
func (s *Server) StopContainer(ctx context.Context, req *pb.StopContainerRequest) (*pb.StopContainerResponse, error) {
	logrus.Debugf("StopContainer %+v", req)
	c, err := s.getContainerFromRequest(req)
	if err != nil {
		return nil, err
	}

	if err := s.runtime.StopContainer(c); err != nil {
		return nil, fmt.Errorf("failed to stop container %s: %v", c.ID(), err)
	}

	return &pb.StopContainerResponse{}, nil
}

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (s *Server) RemoveContainer(ctx context.Context, req *pb.RemoveContainerRequest) (*pb.RemoveContainerResponse, error) {
	logrus.Debugf("RemoveContainer %+v", req)
	c, err := s.getContainerFromRequest(req)
	if err != nil {
		return nil, err
	}

	if err := s.runtime.UpdateStatus(c); err != nil {
		return nil, fmt.Errorf("failed to update container state: %v", err)
	}

	cState := s.runtime.ContainerStatus(c)
	if cState.Status == oci.ContainerStateCreated || cState.Status == oci.ContainerStateRunning {
		if err := s.runtime.StopContainer(c); err != nil {
			return nil, fmt.Errorf("failed to stop container %s: %v", c.ID(), err)
		}
	}

	if err := s.runtime.DeleteContainer(c); err != nil {
		return nil, fmt.Errorf("failed to delete container %s: %v", c.ID(), err)
	}

	containerDir := filepath.Join(s.runtime.ContainerDir(), c.ID())
	if err := os.RemoveAll(containerDir); err != nil {
		return nil, fmt.Errorf("failed to remove container %s directory: %v", c.ID(), err)
	}

	s.releaseContainerName(c.Name())
	s.removeContainer(c)

	if err := s.ctrIDIndex.Delete(c.ID()); err != nil {
		return nil, err
	}

	return &pb.RemoveContainerResponse{}, nil
}

// filterContainer returns whether passed container matches filtering criteria
func filterContainer(c *pb.Container, filter *pb.ContainerFilter) bool {
	if filter != nil {
		if filter.State != nil {
			if *c.State != *filter.State {
				return false
			}
		}
		if filter.LabelSelector != nil {
			sel := fields.SelectorFromSet(filter.LabelSelector)
			if !sel.Matches(fields.Set(c.Labels)) {
				return false
			}
		}
	}
	return true
}

// ListContainers lists all containers by filters.
func (s *Server) ListContainers(ctx context.Context, req *pb.ListContainersRequest) (*pb.ListContainersResponse, error) {
	logrus.Debugf("ListContainers %+v", req)
	var ctrs []*pb.Container
	filter := req.Filter
	ctrList := s.state.containers.List()

	// Filter using container id and pod id first.
	if filter != nil {
		if filter.Id != nil {
			c := s.state.containers.Get(*filter.Id)
			if c != nil {
				if filter.PodSandboxId != nil {
					if c.Sandbox() == *filter.PodSandboxId {
						ctrList = []*oci.Container{c}
					} else {
						ctrList = []*oci.Container{}
					}

				} else {
					ctrList = []*oci.Container{c}
				}
			}
		} else {
			if filter.PodSandboxId != nil {
				pod := s.state.sandboxes[*filter.PodSandboxId]
				if pod == nil {
					ctrList = []*oci.Container{}
				} else {
					ctrList = pod.containers.List()
				}
			}
		}
	}

	for _, ctr := range ctrList {
		if err := s.runtime.UpdateStatus(ctr); err != nil {
			return nil, err
		}

		podSandboxID := ctr.Sandbox()
		cState := s.runtime.ContainerStatus(ctr)
		created := cState.Created.Unix()
		rState := pb.ContainerState_UNKNOWN
		cID := ctr.ID()

		c := &pb.Container{
			Id:           &cID,
			PodSandboxId: &podSandboxID,
			CreatedAt:    int64Ptr(created),
			Labels:       ctr.Labels(),
		}

		switch cState.Status {
		case oci.ContainerStateCreated:
			rState = pb.ContainerState_CREATED
		case oci.ContainerStateRunning:
			rState = pb.ContainerState_RUNNING
		case oci.ContainerStateStopped:
			rState = pb.ContainerState_EXITED
		}
		c.State = &rState

		// Filter by other criteria such as state and labels.
		if filterContainer(c, req.Filter) {
			ctrs = append(ctrs, c)
		}
	}

	return &pb.ListContainersResponse{
		Containers: ctrs,
	}, nil
}

// ContainerStatus returns status of the container.
func (s *Server) ContainerStatus(ctx context.Context, req *pb.ContainerStatusRequest) (*pb.ContainerStatusResponse, error) {
	logrus.Debugf("ContainerStatus %+v", req)
	c, err := s.getContainerFromRequest(req)
	if err != nil {
		return nil, err
	}

	if err := s.runtime.UpdateStatus(c); err != nil {
		return nil, err
	}

	containerID := c.ID()
	csr := &pb.ContainerStatusResponse{
		Status: &pb.ContainerStatus{
			Id: &containerID,
		},
	}

	cState := s.runtime.ContainerStatus(c)
	rStatus := pb.ContainerState_UNKNOWN

	switch cState.Status {
	case oci.ContainerStateCreated:
		rStatus = pb.ContainerState_CREATED
		created := cState.Created.Unix()
		csr.Status.CreatedAt = int64Ptr(created)
	case oci.ContainerStateRunning:
		rStatus = pb.ContainerState_RUNNING
		created := cState.Created.Unix()
		csr.Status.CreatedAt = int64Ptr(created)
		started := cState.Started.Unix()
		csr.Status.StartedAt = int64Ptr(started)
	case oci.ContainerStateStopped:
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

// UpdateRuntimeConfig updates the configuration of a running container.
func (s *Server) UpdateRuntimeConfig(ctx context.Context, req *pb.UpdateRuntimeConfigRequest) (*pb.UpdateRuntimeConfigResponse, error) {
	return nil, nil
}

// ExecSync runs a command in a container synchronously.
func (s *Server) ExecSync(ctx context.Context, req *pb.ExecSyncRequest) (*pb.ExecSyncResponse, error) {
	return nil, nil
}

// Exec prepares a streaming endpoint to execute a command in the container.
func (s *Server) Exec(ctx context.Context, req *pb.ExecRequest) (*pb.ExecResponse, error) {
	return nil, nil
}

// Attach prepares a streaming endpoint to attach to a running container.
func (s *Server) Attach(ctx context.Context, req *pb.AttachRequest) (*pb.AttachResponse, error) {
	return nil, nil
}

// PortForward prepares a streaming endpoint to forward ports from a PodSandbox.
func (s *Server) PortForward(ctx context.Context, req *pb.PortForwardRequest) (*pb.PortForwardResponse, error) {
	return nil, nil
}
