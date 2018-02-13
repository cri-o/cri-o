package server

import (
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

// StopContainer stops a running container with a grace period (i.e., timeout).
func (s *Server) StopContainer(ctx context.Context, req *pb.StopContainerRequest) (resp *pb.StopContainerResponse, err error) {
	const operation = "stop_container"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()
	logrus.Debugf("StopContainerRequest %+v", req)

	if err := s.addContainerToWatcherIgnoreList(req.ContainerId); err != nil {
		return nil, err
	}
	defer s.removeContainerFromWatcherIgnoreList(req.ContainerId)

	_, err = s.ContainerServer.ContainerStop(ctx, req.ContainerId, req.Timeout)
	if err != nil {
		return nil, err
	}

	resp = &pb.StopContainerResponse{}
	logrus.Debugf("StopContainerResponse %s: %+v", req.ContainerId, resp)
	return resp, nil
}
