package server

import (
	"golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// CreateContainer creates a new container in specified PodSandbox
func (s *Server) CreateContainer(ctx context.Context, req *pb.CreateContainerRequest) (res *pb.CreateContainerResponse, err error) {
	logrus.Debugf("CreateContainerRequest %+v", req)

	containerID, err := s.manager.CreateContainer(req.GetPodSandboxId(), req.GetConfig(), req.GetSandboxConfig())
	if err != nil {
		return nil, err
	}

	resp := &pb.CreateContainerResponse{
		ContainerId: &containerID,
	}
	logrus.Debugf("CreateContainerResponse: %+v", resp)

	return resp, nil
}

// ListContainers lists all containers by filters.
func (s *Server) ListContainers(ctx context.Context, req *pb.ListContainersRequest) (*pb.ListContainersResponse, error) {
	logrus.Debugf("ListContainersRequest %+v", req)

	ctrs, err := s.manager.ListContainers(req.GetFilter())
	if err != nil {
		return nil, err
	}

	resp := &pb.ListContainersResponse{
		Containers: ctrs,
	}
	logrus.Debugf("ListContainersResponse: %+v", resp)

	return resp, nil
}

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (s *Server) RemoveContainer(ctx context.Context, req *pb.RemoveContainerRequest) (*pb.RemoveContainerResponse, error) {
	logrus.Debugf("RemoveContainerRequest %+v", req)

	if err := s.manager.RemoveContainer(req.GetContainerId()); err != nil {
		return nil, err
	}

	resp := &pb.RemoveContainerResponse{}
	logrus.Debugf("RemoveContainerResponse: %+v", resp)

	return resp, nil
}

// StartContainer starts the container.
func (s *Server) StartContainer(ctx context.Context, req *pb.StartContainerRequest) (*pb.StartContainerResponse, error) {
	logrus.Debugf("StartContainerRequest %+v", req)

	if err := s.manager.StartContainer(req.GetContainerId()); err != nil {
		return nil, err
	}

	resp := &pb.StartContainerResponse{}
	logrus.Debugf("StartContainerResponse %+v", resp)

	return resp, nil
}

// ContainerStatus returns status of the container.
func (s *Server) ContainerStatus(ctx context.Context, req *pb.ContainerStatusRequest) (*pb.ContainerStatusResponse, error) {
	logrus.Debugf("ContainerStatusRequest %+v", req)

	status, err := s.manager.ContainerStatus(req.GetContainerId())
	if err != nil {
		return nil, err
	}

	resp := &pb.ContainerStatusResponse{
		Status: status,
	}
	logrus.Debugf("ContainerStatusResponse: %+v", resp)

	return resp, nil
}

// StopContainer stops a running container with a grace period (i.e., timeout).
func (s *Server) StopContainer(ctx context.Context, req *pb.StopContainerRequest) (*pb.StopContainerResponse, error) {
	logrus.Debugf("StopContainerRequest %+v", req)

	if err := s.manager.StopContainer(req.GetContainerId(), req.GetTimeout()); err != nil {
		return nil, err
	}

	resp := &pb.StopContainerResponse{}
	logrus.Debugf("StopContainerResponse: %+v", resp)
	return resp, nil
}

// UpdateRuntimeConfig updates the configuration of a running container.
func (s *Server) UpdateRuntimeConfig(ctx context.Context, req *pb.UpdateRuntimeConfigRequest) (*pb.UpdateRuntimeConfigResponse, error) {
	return nil, nil
}
