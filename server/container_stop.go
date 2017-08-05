package server

import (
	"fmt"

	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// StopContainer stops a running container with a grace period (i.e., timeout).
func (s *Server) StopContainer(ctx context.Context, req *pb.StopContainerRequest) (*pb.StopContainerResponse, error) {
	logrus.Debugf("StopContainerRequest %+v", req)
	c, err := s.GetContainerFromRequest(req.ContainerId)
	if err != nil {
		return nil, err
	}

	if err := s.Runtime().UpdateStatus(c); err != nil {
		return nil, err
	}
	cStatus := s.Runtime().ContainerStatus(c)
	if cStatus.Status != oci.ContainerStateStopped {
		if err := s.Runtime().StopContainer(c, req.Timeout); err != nil {
			return nil, fmt.Errorf("failed to stop container %s: %v", c.ID(), err)
		}
		if err := s.StorageRuntimeServer().StopContainer(c.ID()); err != nil {
			return nil, fmt.Errorf("failed to unmount container %s: %v", c.ID(), err)
		}
	}

	s.ContainerStateToDisk(c)

	resp := &pb.StopContainerResponse{}
	logrus.Debugf("StopContainerResponse: %+v", resp)
	return resp, nil
}
