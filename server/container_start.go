package server

import (
	"fmt"
	"time"

	"github.com/cri-o/cri-o/oci"
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
	state := c.State()
	if state.Status != oci.ContainerStateCreated {
		return nil, fmt.Errorf("container %s is not in created state: %s", c.ID(), state.Status)
	}

	defer func() {
		// if the call to StartContainer fails below we still want to fill
		// some fields of a container status. In particular, we're going to
		// adjust container started/finished time and set an error to be
		// returned in the Reason field for container status call.
		if err != nil {
			c.SetStartFailed(err)
		}
		s.ContainerStateToDisk(c)
	}()

	// in the event a container is created, conmon is oom killed, then the container
	// is started, we want to start the watcher on it, and appropriately kill it.
	// this situation is VERY unlikely, but conmonmon exists because the kernel OOM killer
	// can't be trusted
	if err := s.MonitorConmon(c); err != nil {
		logrus.Debugf("failed to add conmon for %s to monitor: %v", c.ID(), err)
	}

	err = s.Runtime().StartContainer(c)
	if err != nil {
		return nil, fmt.Errorf("failed to start container %s: %v", c.ID(), err)
	}

	logrus.Infof("Started container %s: %s", c.ID(), c.Description())
	resp = &pb.StartContainerResponse{}
	logrus.Debugf("StartContainerResponse %+v", resp)
	return resp, nil
}
