package server

import (
	"fmt"
	"time"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// ReopenContainerLog reopens the containers log file
func (s *Server) ReopenContainerLog(ctx context.Context, req *pb.ReopenContainerLogRequest) (resp *pb.ReopenContainerLogResponse, err error) {
	const operation = "container_reopen_log"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	logrus.Debugf("ReopenContainerLogRequest %+v", req)
	containerID := req.ContainerId
	c := s.GetContainer(containerID)

	if c == nil {
		return nil, fmt.Errorf("could not find container %q", containerID)
	}

	if err := s.ContainerServer.Runtime().UpdateContainerStatus(c); err != nil {
		return nil, err
	}

	cState := c.State()
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return nil, fmt.Errorf("container is not created or running")
	}

	err = s.ContainerServer.Runtime().ReopenContainerLog(c)
	if err == nil {
		resp = &pb.ReopenContainerLogResponse{}
	}

	logrus.Debugf("ReopenContainerLogResponse %s: %+v", containerID, resp)
	return resp, err
}
