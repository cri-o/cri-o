package server

import (
	"fmt"
	"os"
	"path/filepath"

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

	if err := os.MkdirAll(s.runtime.SandboxDir(), 0755); err != nil {
		return nil, err
	}

	// process req.Name
	name := req.GetConfig().GetName()
	if name == "" {
		return nil, fmt.Errorf("PodSandboxConfig.Name should not be empty")
	}

	podSandboxDir := filepath.Join(s.runtime.SandboxDir(), name)
	if _, err := os.Stat(podSandboxDir); err == nil {
		return nil, fmt.Errorf("pod sandbox (%s) already exists", podSandboxDir)
	}

	if err := os.MkdirAll(podSandboxDir, 0755); err != nil {
		return nil, err
	}

	// creates a spec Generator with the default spec.
	g := generate.New()

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
	if err := parseDNSOptions(dnsServers, dnsSearches, resolvPath); err != nil {
		if err1 := removeFile(resolvPath); err1 != nil {
			return nil, fmt.Errorf("%v; failed to remove %s: %v", err, resolvPath, err1)
		}
		return nil, err
	}

	if err := g.AddBindMount(fmt.Sprintf("%s:/etc/resolv.conf", resolvPath)); err != nil {
		return nil, err
	}

	labels := req.GetConfig().GetLabels()
	s.sandboxes = append(s.sandboxes, &sandbox{
		name:   name,
		logDir: logDir,
		labels: labels,
	})

	annotations := req.GetConfig().GetAnnotations()
	for k, v := range annotations {
		err := g.AddAnnotation(fmt.Sprintf("%s=%s", k, v))
		if err != nil {
			return nil, err
		}
	}

	// TODO: double check cgroupParent.
	cgroupParent := req.GetConfig().GetLinux().GetCgroupParent()
	if cgroupParent != "" {
		g.SetLinuxCgroupsPath(cgroupParent)
	}

	// set up namespaces
	if req.GetConfig().GetLinux().GetNamespaceOptions().GetHostNetwork() == false {
		err := g.AddOrReplaceLinuxNamespace("network", "")
		if err != nil {
			return nil, err
		}
	}

	if req.GetConfig().GetLinux().GetNamespaceOptions().GetHostPid() == false {
		err := g.AddOrReplaceLinuxNamespace("pid", "")
		if err != nil {
			return nil, err
		}
	}

	if req.GetConfig().GetLinux().GetNamespaceOptions().GetHostIpc() == false {
		err := g.AddOrReplaceLinuxNamespace("ipc", "")
		if err != nil {
			return nil, err
		}
	}

	err = g.SaveToFile(filepath.Join(podSandboxDir, "config.json"))
	if err != nil {
		return nil, err
	}

	return &pb.CreatePodSandboxResponse{PodSandboxId: &name}, nil
}

// StopPodSandbox stops the sandbox. If there are any running containers in the
// sandbox, they should be force terminated.
func (s *Server) StopPodSandbox(context.Context, *pb.StopPodSandboxRequest) (*pb.StopPodSandboxResponse, error) {
	return nil, nil
}

// DeletePodSandbox deletes the sandbox. If there are any running containers in the
// sandbox, they should be force deleted.
func (s *Server) DeletePodSandbox(context.Context, *pb.DeletePodSandboxRequest) (*pb.DeletePodSandboxResponse, error) {
	return nil, nil
}

// PodSandboxStatus returns the Status of the PodSandbox.
func (s *Server) PodSandboxStatus(context.Context, *pb.PodSandboxStatusRequest) (*pb.PodSandboxStatusResponse, error) {
	return nil, nil
}

// ListPodSandbox returns a list of SandBox.
func (s *Server) ListPodSandbox(context.Context, *pb.ListPodSandboxRequest) (*pb.ListPodSandboxResponse, error) {
	return nil, nil
}

// CreateContainer creates a new container in specified PodSandbox
func (s *Server) CreateContainer(context.Context, *pb.CreateContainerRequest) (*pb.CreateContainerResponse, error) {
	return nil, nil
}

// StartContainer starts the container.
func (s *Server) StartContainer(context.Context, *pb.StartContainerRequest) (*pb.StartContainerResponse, error) {
	return nil, nil
}

// StopContainer stops a running container with a grace period (i.e., timeout).
func (s *Server) StopContainer(context.Context, *pb.StopContainerRequest) (*pb.StopContainerResponse, error) {
	return nil, nil
}

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (s *Server) RemoveContainer(context.Context, *pb.RemoveContainerRequest) (*pb.RemoveContainerResponse, error) {
	return nil, nil
}

// ListContainers lists all containers by filters.
func (s *Server) ListContainers(context.Context, *pb.ListContainersRequest) (*pb.ListContainersResponse, error) {
	return nil, nil
}

// ContainerStatus returns status of the container.
func (s *Server) ContainerStatus(context.Context, *pb.ContainerStatusRequest) (*pb.ContainerStatusResponse, error) {
	return nil, nil
}

// Exec executes the command in the container.
func (s *Server) Exec(pb.RuntimeService_ExecServer) error {
	return nil
}
