package server

import (
	"fmt"

	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (s *Server) RemoveContainer(ctx context.Context, req *pb.RemoveContainerRequest) (*pb.RemoveContainerResponse, error) {
	logrus.Debugf("RemoveContainerRequest %+v", req)
	c, err := s.GetContainerFromRequest(req.ContainerId)
	if err != nil {
		return nil, err
	}

	if err := s.Runtime().UpdateStatus(c); err != nil {
		return nil, fmt.Errorf("failed to update container state: %v", err)
	}

	cState := s.Runtime().ContainerStatus(c)
	if cState.Status == oci.ContainerStateCreated || cState.Status == oci.ContainerStateRunning {
		if err := s.Runtime().StopContainer(c, -1); err != nil {
			return nil, fmt.Errorf("failed to stop container %s: %v", c.ID(), err)
		}
		if err := s.StorageRuntimeServer().StopContainer(c.ID()); err != nil {
			return nil, fmt.Errorf("failed to unmount container %s: %v", c.ID(), err)
		}
	}

	if err := s.Runtime().DeleteContainer(c); err != nil {
		return nil, fmt.Errorf("failed to delete container %s: %v", c.ID(), err)
	}

	s.removeContainer(c)

	if err := s.StorageRuntimeServer().DeleteContainer(c.ID()); err != nil {
		return nil, fmt.Errorf("failed to delete storage for container %s: %v", c.ID(), err)
	}

	s.ReleaseContainerName(c.Name())

	if err := s.CtrIDIndex().Delete(c.ID()); err != nil {
		return nil, err
	}

	resp := &pb.RemoveContainerResponse{}
	logrus.Debugf("RemoveContainerResponse: %+v", resp)
	return resp, nil
}
