package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/stringid"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/utils"
	"github.com/opencontainers/runc/libcontainer/label"
	"github.com/opencontainers/runtime-tools/generate"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

type sandbox struct {
	id           string
	name         string
	logDir       string
	labels       map[string]string
	annotations  map[string]string
	containers   oci.Store
	processLabel string
	mountLabel   string
}

const (
	podInfraRootfs      = "/var/lib/ocid/graph/vfs/pause"
	podDefaultNamespace = "default"
)

func (s *sandbox) addContainer(c *oci.Container) {
	s.containers.Add(c.Name(), c)
}

func (s *sandbox) getContainer(name string) *oci.Container {
	return s.containers.Get(name)
}

func (s *sandbox) removeContainer(c *oci.Container) {
	s.containers.Delete(c.Name())
}

func (s *Server) generatePodIDandName(name string, namespace string, attempt uint32) (string, string, error) {
	var (
		err error
		id  = stringid.GenerateNonCryptoID()
	)
	if namespace == "" {
		namespace = podDefaultNamespace
	}

	if name, err = s.reservePodName(id, fmt.Sprintf("%s-%s-%v", namespace, name, attempt)); err != nil {
		return "", "", err
	}
	return id, name, err
}

type podSandboxRequest interface {
	GetPodSandboxId() string
}

func (s *Server) getPodSandboxFromRequest(req podSandboxRequest) (*sandbox, error) {
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
	return sb, nil
}

// RunPodSandbox creates and runs a pod-level sandbox.
func (s *Server) RunPodSandbox(ctx context.Context, req *pb.RunPodSandboxRequest) (*pb.RunPodSandboxResponse, error) {
	var processLabel, mountLabel string
	// process req.Name
	name := req.GetConfig().GetMetadata().GetName()
	if name == "" {
		return nil, fmt.Errorf("PodSandboxConfig.Name should not be empty")
	}

	namespace := req.GetConfig().GetMetadata().GetNamespace()
	attempt := req.GetConfig().GetMetadata().GetAttempt()

	var err error
	id, name, err := s.generatePodIDandName(name, namespace, attempt)
	if err != nil {
		return nil, err
	}
	podSandboxDir := filepath.Join(s.sandboxDir, id)
	if _, err = os.Stat(podSandboxDir); err == nil {
		return nil, fmt.Errorf("pod sandbox (%s) already exists", podSandboxDir)
	}

	if err = os.MkdirAll(podSandboxDir, 0755); err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			if err2 := os.RemoveAll(podSandboxDir); err2 != nil {
				logrus.Warnf("couldn't cleanup podSandboxDir %s: %v", podSandboxDir, err2)
			}
		}
	}()

	// creates a spec Generator with the default spec.
	g := generate.New()

	podInfraRootfs := filepath.Join(s.root, "graph/vfs/pause")
	// setup defaults for the pod sandbox
	g.SetRootPath(filepath.Join(podInfraRootfs, "rootfs"))
	g.SetRootReadonly(true)
	g.SetProcessArgs([]string{"/pause"})

	// set hostname
	hostname := req.GetConfig().GetHostname()
	if hostname != "" {
		g.SetHostname(hostname)
	}

	// set log directory
	logDir := req.GetConfig().GetLogDirectory()
	if logDir == "" {
		logDir = fmt.Sprintf("/var/log/ocid/pods/%s", id)
	}

	// set DNS options
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

	// add labels
	labels := req.GetConfig().GetLabels()
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, err
	}

	// add annotations
	annotations := req.GetConfig().GetAnnotations()
	annotationsJSON, err := json.Marshal(annotations)
	if err != nil {
		return nil, err
	}

	processLabel, mountLabel, err = getSELinuxLabels(nil)
	if err != nil {
		return nil, err
	}

	containerID, containerName, err := s.generateContainerIDandName(name, "infra", 0)
	g.AddAnnotation("ocid/labels", string(labelsJSON))
	g.AddAnnotation("ocid/annotations", string(annotationsJSON))
	g.AddAnnotation("ocid/log_path", logDir)
	g.AddAnnotation("ocid/name", name)
	g.AddAnnotation("ocid/container_name", containerName)
	g.AddAnnotation("ocid/container_id", containerID)
	s.addSandbox(&sandbox{
		id:           id,
		name:         name,
		logDir:       logDir,
		labels:       labels,
		annotations:  annotations,
		containers:   oci.NewMemoryStore(),
		processLabel: processLabel,
		mountLabel:   mountLabel,
	})

	for k, v := range annotations {
		g.AddAnnotation(k, v)
	}

	// setup cgroup settings
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

	if _, err = os.Stat(podInfraRootfs); err != nil {
		if os.IsNotExist(err) {
			// TODO: Replace by rootfs creation API when it is ready
			if err = utils.CreateInfraRootfs(podInfraRootfs, s.pausePath); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	container, err := oci.NewContainer(containerID, containerName, podSandboxDir, podSandboxDir, labels, id, false)
	if err != nil {
		return nil, err
	}

	if err = s.runtime.CreateContainer(container); err != nil {
		return nil, err
	}

	if err = s.runtime.UpdateStatus(container); err != nil {
		return nil, err
	}

	// setup the network
	podNamespace := ""
	netnsPath, err := container.NetNsPath()
	if err != nil {
		return nil, err
	}
	if err = s.netPlugin.SetUpPod(netnsPath, podNamespace, id, containerName); err != nil {
		return nil, fmt.Errorf("failed to create network for container %s in sandbox %s: %v", containerName, id, err)
	}

	if err = s.runtime.StartContainer(container); err != nil {
		return nil, err
	}

	s.addContainer(container)

	if err = s.podIDIndex.Add(id); err != nil {
		return nil, err
	}

	if err = s.runtime.UpdateStatus(container); err != nil {
		return nil, err
	}

	return &pb.RunPodSandboxResponse{PodSandboxId: &id}, nil
}

// StopPodSandbox stops the sandbox. If there are any running containers in the
// sandbox, they should be force terminated.
func (s *Server) StopPodSandbox(ctx context.Context, req *pb.StopPodSandboxRequest) (*pb.StopPodSandboxResponse, error) {
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

	podInfraContainer := sb.name + "-infra"
	for _, c := range sb.containers.List() {
		if podInfraContainer == c.Name() {
			podNamespace := ""
			netnsPath, err := c.NetNsPath()
			if err != nil {
				return nil, err
			}
			if err := s.netPlugin.TearDownPod(netnsPath, podNamespace, sandboxID, podInfraContainer); err != nil {
				return nil, fmt.Errorf("failed to destroy network for container %s in sandbox %s: %v", c.Name(), sandboxID, err)
			}
		}
		cStatus := s.runtime.ContainerStatus(c)
		if cStatus.Status != "stopped" {
			if err := s.runtime.StopContainer(c); err != nil {
				return nil, fmt.Errorf("failed to stop container %s in sandbox %s: %v", c.Name(), sandboxID, err)
			}
		}
	}

	return &pb.StopPodSandboxResponse{}, nil
}

// RemovePodSandbox deletes the sandbox. If there are any running containers in the
// sandbox, they should be force deleted.
func (s *Server) RemovePodSandbox(ctx context.Context, req *pb.RemovePodSandboxRequest) (*pb.RemovePodSandboxResponse, error) {
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

	podInfraContainerName := sb.name + "-infra"
	var podInfraContainer *oci.Container

	// Delete all the containers in the sandbox
	for _, c := range sb.containers.List() {
		if err := s.runtime.UpdateStatus(c); err != nil {
			return nil, fmt.Errorf("failed to update container state: %v", err)
		}

		cState := s.runtime.ContainerStatus(c)
		if cState.Status == ContainerStateCreated || cState.Status == ContainerStateRunning {
			if err := s.runtime.StopContainer(c); err != nil {
				return nil, fmt.Errorf("failed to stop container %s: %v", c.Name(), err)
			}
		}

		if err := s.runtime.DeleteContainer(c); err != nil {
			return nil, fmt.Errorf("failed to delete container %s in sandbox %s: %v", c.Name(), sandboxID, err)
		}

		if podInfraContainerName == c.Name() {
			podInfraContainer = c
			continue
		}

		containerDir := filepath.Join(s.runtime.ContainerDir(), c.Name())
		if err := os.RemoveAll(containerDir); err != nil {
			return nil, fmt.Errorf("failed to remove container %s directory: %v", c.Name(), err)
		}

		s.releaseContainerName(c.Name())
		s.removeContainer(c)
	}

	// Remove the files related to the sandbox
	podSandboxDir := filepath.Join(s.sandboxDir, sandboxID)
	if err := os.RemoveAll(podSandboxDir); err != nil {
		return nil, fmt.Errorf("failed to remove sandbox %s directory: %v", sandboxID, err)
	}
	s.releaseContainerName(podInfraContainer.Name())
	s.removeContainer(podInfraContainer)

	s.releasePodName(sb.name)
	s.removeSandbox(sandboxID)

	return &pb.RemovePodSandboxResponse{}, nil
}

// PodSandboxStatus returns the Status of the PodSandbox.
func (s *Server) PodSandboxStatus(ctx context.Context, req *pb.PodSandboxStatusRequest) (*pb.PodSandboxStatusResponse, error) {
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

	podInfraContainerName := sb.name + "-infra"
	podInfraContainer := sb.getContainer(podInfraContainerName)
	if err = s.runtime.UpdateStatus(podInfraContainer); err != nil {
		return nil, err
	}

	cState := s.runtime.ContainerStatus(podInfraContainer)
	created := cState.Created.Unix()

	netNsPath, err := podInfraContainer.NetNsPath()
	if err != nil {
		return nil, err
	}
	podNamespace := ""
	ip, err := s.netPlugin.GetContainerNetworkStatus(netNsPath, podNamespace, sandboxID, podInfraContainerName)
	if err != nil {
		// ignore the error on network status
		ip = ""
	}

	rStatus := pb.PodSandBoxState_NOTREADY
	if cState.Status == ContainerStateRunning {
		rStatus = pb.PodSandBoxState_READY
	}

	return &pb.PodSandboxStatusResponse{
		Status: &pb.PodSandboxStatus{
			Id:        &sbID,
			CreatedAt: int64Ptr(created),
			Linux: &pb.LinuxPodSandboxStatus{
				Namespaces: &pb.Namespace{
					Network: sPtr(netNsPath),
				},
			},
			Network: &pb.PodSandboxNetworkStatus{Ip: &ip},
			State:   &rStatus,
		},
	}, nil
}

// ListPodSandbox returns a list of SandBoxes.
func (s *Server) ListPodSandbox(context.Context, *pb.ListPodSandboxRequest) (*pb.ListPodSandboxResponse, error) {
	var pods []*pb.PodSandbox
	for _, sb := range s.state.sandboxes {
		podInfraContainerName := sb.name + "-infra"
		podInfraContainer := sb.getContainer(podInfraContainerName)
		if podInfraContainer == nil {
			// this can't really happen, but if it does because of a bug
			// it's better not to panic
			continue
		}
		if err := s.runtime.UpdateStatus(podInfraContainer); err != nil {
			return nil, err
		}
		cState := s.runtime.ContainerStatus(podInfraContainer)
		created := cState.Created.Unix()
		rStatus := pb.PodSandBoxState_NOTREADY
		if cState.Status == ContainerStateRunning {
			rStatus = pb.PodSandBoxState_READY
		}

		pod := &pb.PodSandbox{
			Id:        &sb.id,
			CreatedAt: int64Ptr(created),
			State:     &rStatus,
		}

		pods = append(pods, pod)
	}

	return &pb.ListPodSandboxResponse{
		Items: pods,
	}, nil
}

func getSELinuxLabels(selinuxOptions *pb.SELinuxOption) (processLabel string, mountLabel string, err error) {
	processLabel = ""
	if selinuxOptions != nil {
		user := selinuxOptions.GetUser()
		if user == "" {
			return "", "", fmt.Errorf("SELinuxOption.User is empty")
		}

		role := selinuxOptions.GetRole()
		if role == "" {
			return "", "", fmt.Errorf("SELinuxOption.Role is empty")
		}

		t := selinuxOptions.GetType()
		if t == "" {
			return "", "", fmt.Errorf("SELinuxOption.Type is empty")
		}

		level := selinuxOptions.GetLevel()
		if level == "" {
			return "", "", fmt.Errorf("SELinuxOption.Level is empty")
		}
		processLabel = fmt.Sprintf("%s:%s:%s:%s", user, role, t, level)
	}
	return label.InitLabels(label.DupSecOpt(processLabel))
}
