package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/stringid"
	"github.com/kubernetes-incubator/ocid/oci"
	"github.com/kubernetes-incubator/ocid/utils"
	"github.com/opencontainers/runtime-tools/generate"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

type sandbox struct {
	id         string
	name       string
	logDir     string
	labels     map[string]string
	containers oci.Store
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

func (s *Server) generatePodIDandName(name, namespace string) (string, string, error) {
	var (
		err error
		id  = stringid.GenerateNonCryptoID()
	)
	if namespace == "" {
		namespace = podDefaultNamespace
	}
	if name, err = s.reservePodName(id, namespace+"-"+name); err != nil {
		return "", "", err
	}
	return id, name, err
}

// CreatePodSandbox creates a pod-level sandbox.
func (s *Server) CreatePodSandbox(ctx context.Context, req *pb.CreatePodSandboxRequest) (*pb.CreatePodSandboxResponse, error) {
	// process req.Name
	name := req.GetConfig().GetMetadata().GetName()
	if name == "" {
		return nil, fmt.Errorf("PodSandboxConfig.Name should not be empty")
	}

	namespace := req.GetConfig().GetMetadata().GetNamespace()

	var err error
	id, name, err := s.generatePodIDandName(name, namespace)
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
	g.AddAnnotation("ocid/labels", string(labelsJSON))
	g.AddAnnotation("ocid/log_path", logDir)
	g.AddAnnotation("ocid/name", name)
	containerName := name + "-infra"
	g.AddAnnotation("ocid/container_name", containerName)
	s.addSandbox(&sandbox{
		id:         id,
		name:       name,
		logDir:     logDir,
		labels:     labels,
		containers: oci.NewMemoryStore(),
	})

	annotations := req.GetConfig().GetAnnotations()
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
			if err := utils.CreateFakeRootfs(podInfraRootfs, "docker://kubernetes/pause"); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	container, err := oci.NewContainer(containerName, podSandboxDir, podSandboxDir, labels, id, false)
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

	return &pb.CreatePodSandboxResponse{PodSandboxId: &id}, nil
}

// StopPodSandbox stops the sandbox. If there are any running containers in the
// sandbox, they should be force terminated.
func (s *Server) StopPodSandbox(ctx context.Context, req *pb.StopPodSandboxRequest) (*pb.StopPodSandboxResponse, error) {
	sbID := req.PodSandboxId
	if *sbID == "" {
		return nil, fmt.Errorf("PodSandboxId should not be empty")
	}
	sb := s.getSandbox(*sbID)
	if sb == nil {
		return nil, fmt.Errorf("specified sandbox not found: %s", *sbID)
	}

	podInfraContainer := sb.name + "-infra"
	for _, c := range sb.containers.List() {
		if podInfraContainer == c.Name() {
			podNamespace := ""
			netnsPath, err := c.NetNsPath()
			if err != nil {
				return nil, err
			}
			if err := s.netPlugin.TearDownPod(netnsPath, podNamespace, *sbID, podInfraContainer); err != nil {
				return nil, fmt.Errorf("failed to destroy network for container %s in sandbox %s: %v", c.Name(), *sbID, err)
			}
		}
		cStatus := s.runtime.ContainerStatus(c)
		if cStatus.Status != "stopped" {
			if err := s.runtime.StopContainer(c); err != nil {
				return nil, fmt.Errorf("failed to stop container %s in sandbox %s: %v", c.Name(), *sbID, err)
			}
		}
	}

	return &pb.StopPodSandboxResponse{}, nil
}

// RemovePodSandbox deletes the sandbox. If there are any running containers in the
// sandbox, they should be force deleted.
func (s *Server) RemovePodSandbox(ctx context.Context, req *pb.RemovePodSandboxRequest) (*pb.RemovePodSandboxResponse, error) {
	sbID := req.PodSandboxId
	if *sbID == "" {
		return nil, fmt.Errorf("PodSandboxId should not be empty")
	}
	sb := s.getSandbox(*sbID)
	if sb == nil {
		return nil, fmt.Errorf("specified sandbox not found: %s", *sbID)
	}

	podInfraContainer := sb.name + "-infra"

	// Delete all the containers in the sandbox
	for _, c := range sb.containers.List() {
		if err := s.runtime.DeleteContainer(c); err != nil {
			return nil, fmt.Errorf("failed to delete container %s in sandbox %s: %v", c.Name(), *sbID, err)
		}
		if podInfraContainer == c.Name() {
			continue
		}
		containerDir := filepath.Join(s.runtime.ContainerDir(), c.Name())
		if err := os.RemoveAll(containerDir); err != nil {
			return nil, fmt.Errorf("failed to remove container %s directory: %v", c.Name(), err)
		}
	}

	// Remove the files related to the sandbox
	podSandboxDir := filepath.Join(s.sandboxDir, *sbID)
	if err := os.RemoveAll(podSandboxDir); err != nil {
		return nil, fmt.Errorf("failed to remove sandbox %s directory: %v", *sbID, err)
	}

	return &pb.RemovePodSandboxResponse{}, nil
}

// PodSandboxStatus returns the Status of the PodSandbox.
func (s *Server) PodSandboxStatus(ctx context.Context, req *pb.PodSandboxStatusRequest) (*pb.PodSandboxStatusResponse, error) {
	sbID := req.PodSandboxId
	if *sbID == "" {
		return nil, fmt.Errorf("PodSandboxId should not be empty")
	}
	sb := s.getSandbox(*sbID)
	if sb == nil {
		return nil, fmt.Errorf("specified sandbox not found: %s", *sbID)
	}

	podInfraContainerName := sb.name + "-infra"
	podInfraContainer := sb.getContainer(podInfraContainerName)

	cState := s.runtime.ContainerStatus(podInfraContainer)
	created := cState.Created.Unix()

	netNsPath, err := podInfraContainer.NetNsPath()
	if err != nil {
		return nil, err
	}
	podNamespace := ""
	ip, err := s.netPlugin.GetContainerNetworkStatus(netNsPath, podNamespace, *sbID, podInfraContainerName)
	if err != nil {
		// ignore the error on network status
		ip = ""
	}

	return &pb.PodSandboxStatusResponse{
		Status: &pb.PodSandboxStatus{
			Id:        sbID,
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
