package server

import (
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (s *Server) RemoveContainer(ctx context.Context, req *pb.RemoveContainerRequest) (resp *pb.RemoveContainerResponse, err error) {
	const operation = "remove_container"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()
	logrus.Debugf("RemoveContainerRequest: %+v", req)

	// save container description to print
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, err
	}

	_, err = s.ContainerServer.Remove(ctx, req.ContainerId, true)
	if err != nil {
		return nil, err
	}

	s.StopMonitoringConmon(c)

	logrus.Infof("Removed container %s: %s", c.ID(), c.Description())
	resp = &pb.RemoveContainerResponse{}
	logrus.Debugf("RemoveContainerResponse: %+v", resp)
	return resp, nil
}
