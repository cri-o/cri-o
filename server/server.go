package server

import (
	pb "github.com/kubernetes/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	"golang.org/x/net/context"
)

// Server implements the RuntimeService
type Server struct{}

// Version returns the runtime name, runtime version and runtime API version
func (s *Server) Version(context.Context, *pb.VersionRequest) (*pb.VersionResponse, error) {
	return nil, nil
}

// CreatePodSandbox creates a pod-level sandbox.
// The definition of PodSandbox is at https://github.com/kubernetes/kubernetes/pull/25899
func (s *Server) CreatePodSandbox(context.Context, *pb.CreatePodSandboxRequest) (*pb.CreatePodSandboxResponse, error) {
	return nil, nil
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
