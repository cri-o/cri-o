package server

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (s *Server) RemoveContainer(ctx context.Context, req *pb.RemoveContainerRequest) (*pb.RemoveContainerResponse, error) {
	logrus.Debugf("RemoveContainerRequest %+v", req)
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

	resp := &pb.RemoveContainerResponse{}
	logrus.Debugf("RemoveContainerResponse: %+v", resp)
	return resp, nil
}
