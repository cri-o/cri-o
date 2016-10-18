package server

import (
	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// ContainerStatus returns status of the container.
func (s *Server) ContainerStatus(ctx context.Context, req *pb.ContainerStatusRequest) (*pb.ContainerStatusResponse, error) {
	logrus.Debugf("ContainerStatusRequest %+v", req)
	s.Update()
	c, err := s.getContainerFromRequest(req)
	if err != nil {
		return nil, err
	}

	if err := s.runtime.UpdateStatus(c); err != nil {
		return nil, err
	}

	containerID := c.ID()
	resp := &pb.ContainerStatusResponse{
		Status: &pb.ContainerStatus{
			Id:       &containerID,
			Metadata: c.Metadata(),
		},
	}

	cState := s.runtime.ContainerStatus(c)
	rStatus := pb.ContainerState_CONTAINER_UNKNOWN

	switch cState.Status {
	case oci.ContainerStateCreated:
		rStatus = pb.ContainerState_CONTAINER_CREATED
		created := cState.Created.UnixNano()
		resp.Status.CreatedAt = int64Ptr(created)
	case oci.ContainerStateRunning:
		rStatus = pb.ContainerState_CONTAINER_RUNNING
		created := cState.Created.UnixNano()
		resp.Status.CreatedAt = int64Ptr(created)
		started := cState.Started.UnixNano()
		resp.Status.StartedAt = int64Ptr(started)
	case oci.ContainerStateStopped:
		rStatus = pb.ContainerState_CONTAINER_EXITED
		created := cState.Created.UnixNano()
		resp.Status.CreatedAt = int64Ptr(created)
		started := cState.Started.UnixNano()
		resp.Status.StartedAt = int64Ptr(started)
		finished := cState.Finished.UnixNano()
		resp.Status.FinishedAt = int64Ptr(finished)
		resp.Status.ExitCode = int32Ptr(cState.ExitCode)
	}

	resp.Status.State = &rStatus

	logrus.Debugf("ContainerStatusResponse: %+v", resp)
	return resp, nil
}
