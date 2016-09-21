package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/ocid/oci"
	"github.com/kubernetes-incubator/ocid/utils"
	pb "github.com/kubernetes/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	"github.com/opencontainers/ocitools/generate"
	"golang.org/x/net/context"
)

type sandbox struct {
	name       string
	logDir     string
	labels     map[string]string
	containers oci.Store
}

type metadata struct {
	LogDir        string            `json:"log_dir"`
	ContainerName string            `json:"container_name"`
	Labels        map[string]string `json:"labels"`
}

const (
	podInfraRootfs = "/var/lib/ocid/graph/vfs/pause"
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

// CreatePodSandbox creates a pod-level sandbox.
// The definition of PodSandbox is at https://github.com/kubernetes/kubernetes/pull/25899
func (s *Server) CreatePodSandbox(ctx context.Context, req *pb.CreatePodSandboxRequest) (*pb.CreatePodSandboxResponse, error) {
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

	var err error
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
	g.SetRootPath(podInfraRootfs)
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
		containers: oci.NewMemoryStore(),
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

	meta := &metadata{
		LogDir:        logDir,
		ContainerName: containerName,
		Labels:        labels,
	}

	b, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}

	// TODO: eventually we would track all containers in this pod so on server start
	// we can repopulate the structs in memory properly...
	// e.g. each container can write itself in podSandboxDir
	err = ioutil.WriteFile(filepath.Join(podSandboxDir, "metadata.json"), b, 0644)
	if err != nil {
		return nil, err
	}

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
	for _, c := range sb.containers.List() {
		if podInfraContainer == c.Name() {
			podNamespace := ""
			netnsPath, err := c.NetNsPath()
			if err != nil {
				return nil, err
			}
			if err := s.netPlugin.TearDownPod(netnsPath, podNamespace, *sbName, podInfraContainer); err != nil {
				return nil, fmt.Errorf("failed to destroy network for container %s in sandbox %s: %v", c.Name(), *sbName, err)
			}
		}
		cStatus := s.runtime.ContainerStatus(c)
		if cStatus.Status != "stopped" {
			if err := s.runtime.StopContainer(c); err != nil {
				return nil, fmt.Errorf("failed to stop container %s in sandbox %s: %v", c.Name(), *sbName, err)
			}
		}
	}

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
	for _, c := range sb.containers.List() {
		if err := s.runtime.DeleteContainer(c); err != nil {
			return nil, fmt.Errorf("failed to delete container %s in sandbox %s: %v", c.Name(), *sbName, err)
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
	podSandboxDir := filepath.Join(s.sandboxDir, *sbName)
	if err := os.RemoveAll(podSandboxDir); err != nil {
		return nil, fmt.Errorf("failed to remove sandbox %s directory: %v", *sbName, err)
	}

	return &pb.RemovePodSandboxResponse{}, nil
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
