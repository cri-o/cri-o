package server

import (
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

// StopContainer stops a running container with a grace period (i.e., timeout).
func (s *Server) StopContainer(ctx context.Context, req *pb.StopContainerRequest) (resp *pb.StopContainerResponse, err error) {
	const operation = "stop_container"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()
	logrus.Debugf("StopContainerRequest %+v", req)

	// save container description to print
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, err
	}

	_, err = s.ContainerServer.ContainerStop(ctx, req.ContainerId, req.Timeout)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Stopped container %s: %s", c.ID(), c.Description())
	resp = &pb.StopContainerResponse{}
	logrus.Debugf("StopContainerResponse %s: %+v", req.ContainerId, resp)
	return resp, nil
}
