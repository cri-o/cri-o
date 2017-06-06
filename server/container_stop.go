package server

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1"
)

// StopContainer stops a running container with a grace period (i.e., timeout).
func (s *Server) StopContainer(ctx context.Context, req *pb.StopContainerRequest) (*pb.StopContainerResponse, error) {
	logrus.Debugf("StopContainerRequest %+v", req)
	c, err := s.getContainerFromRequest(req.ContainerId)
	if err != nil {
		return nil, err
	}

	if err := s.runtime.UpdateStatus(c); err != nil {
		return nil, err
	}
	cStatus := s.runtime.ContainerStatus(c)
	if cStatus.Status != oci.ContainerStateStopped {
		if err := s.runtime.StopContainer(c, req.Timeout); err != nil {
			return nil, fmt.Errorf("failed to stop container %s: %v", c.ID(), err)
		}
	}

	s.containerStateToDisk(c)

	resp := &pb.StopContainerResponse{}
	logrus.Debugf("StopContainerResponse: %+v", resp)
	return resp, nil
}
