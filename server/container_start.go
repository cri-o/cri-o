package server

import (
	"fmt"
	"time"

	"github.com/kubernetes-sigs/cri-o/oci"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

// StartContainer starts the container.
func (s *Server) StartContainer(ctx context.Context, req *pb.StartContainerRequest) (resp *pb.StartContainerResponse, err error) {
	const operation = "start_container"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()
	logrus.Debugf("StartContainerRequest %+v", req)
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, err
	}
	state := s.Runtime().ContainerStatus(c)
	if state.Status != oci.ContainerStateCreated {
		return nil, fmt.Errorf("container %s is not in created state: %s", c.ID(), state.Status)
	}

	defer func() {
		// always set container state to disk, especially for failures
		s.ContainerStateToDisk(c)
	}()

	err = s.Runtime().StartContainer(c)
	if err != nil {
		return nil, fmt.Errorf("failed to start container %s: %v", c.ID(), err)
	}

	resp = &pb.StartContainerResponse{}
	logrus.Debugf("StartContainerResponse %+v", resp)
	return resp, nil
}
