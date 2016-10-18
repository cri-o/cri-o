package server

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// StartContainer starts the container.
func (s *Server) StartContainer(ctx context.Context, req *pb.StartContainerRequest) (*pb.StartContainerResponse, error) {
	logrus.Debugf("StartContainerRequest %+v", req)
	s.Update()
	c, err := s.getContainerFromRequest(req)
	if err != nil {
		return nil, err
	}

	if err = s.runtime.StartContainer(c); err != nil {
		return nil, fmt.Errorf("failed to start container %s: %v", c.ID(), err)
	}

	resp := &pb.StartContainerResponse{}
	logrus.Debugf("StartContainerResponse %+v", resp)
	return resp, nil
}
