package server

import (
	"path/filepath"

	pb "github.com/kubernetes/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	"github.com/opencontainers/ocitools/generate"
	"golang.org/x/net/context"
)

// Server implements the RuntimeService
type Server struct{}

// Version returns the runtime name, runtime version and runtime API version
func (s *Server) Version(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
	var err error

	version, err := getGPRCVersion()
	if err != nil {
		return nil, nil
	}

	runtimeName := "runc"

	runtimeVersion, err := execRuncVersion("runc", "-v")
	if err != nil {
		return nil, nil
	}

	runtimeApiVersion := "v1alpha1"

	return &pb.VersionResponse{
		Version:           &version,
		RuntimeName:       &runtimeName,
		RuntimeVersion:    &runtimeVersion,
		RuntimeApiVersion: &runtimeApiVersion,
	}, nil
}

// CreatePodSandbox creates a pod-level sandbox.
// The definition of PodSandbox is at https://github.com/kubernetes/kubernetes/pull/25899
func (s *Server) CreatePodSandbox(ctx context.Context, req *pb.CreatePodSandboxRequest) (*pb.CreatePodSandboxResponse, error) {
	var err error

	// TODO: Parametrize as a global argument to ocid
	ocidSandboxDir := "/var/lib/ocid/sandbox"
	podSandboxDir := filepath.Join(ocidSandboxDir, req.GetConfig().GetName())

	g := generate.New()

	// TODO: Customize the config per the settings in the req
	err = g.SaveToFile(filepath.Join(podSandboxDir, "config.json"))

	return nil, err
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
