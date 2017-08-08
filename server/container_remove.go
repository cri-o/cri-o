package server

import (
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (s *Server) RemoveContainer(ctx context.Context, req *pb.RemoveContainerRequest) (*pb.RemoveContainerResponse, error) {
	_, err := s.ContainerServer.ContainerRemove(req.ContainerId, true)
	if err != nil {
		return nil, err
	}

	resp := &pb.RemoveContainerResponse{}
	logrus.Debugf("RemoveContainerResponse: %+v", resp)
	return resp, nil
}
