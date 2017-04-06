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
	c, err := s.getContainerFromRequest(req.ContainerId)
	if err != nil {
		return nil, err
	}

	if err = s.runtime.UpdateStatus(c); err != nil {
		return nil, err
	}

	containerID := c.ID()
	image := c.Image()
	resp := &pb.ContainerStatusResponse{
		Status: &pb.ContainerStatus{
			Id:          containerID,
			Metadata:    c.Metadata(),
			Labels:      c.Labels(),
			Annotations: c.Annotations(),
			Image:       image,
		},
	}

	status, err := s.images.ImageStatus(s.imageContext, image.Image)
	if err != nil {
		return nil, err
	}

	resp.Status.ImageRef = status.ID

	cState := s.runtime.ContainerStatus(c)
	rStatus := pb.ContainerState_CONTAINER_UNKNOWN

	switch cState.Status {
	case oci.ContainerStateCreated:
		rStatus = pb.ContainerState_CONTAINER_CREATED
		created := cState.Created.UnixNano()
		resp.Status.CreatedAt = created
	case oci.ContainerStateRunning:
		rStatus = pb.ContainerState_CONTAINER_RUNNING
		created := cState.Created.UnixNano()
		resp.Status.CreatedAt = created
		started := cState.Started.UnixNano()
		resp.Status.StartedAt = started
	case oci.ContainerStateStopped:
		rStatus = pb.ContainerState_CONTAINER_EXITED
		created := cState.Created.UnixNano()
		resp.Status.CreatedAt = created
		started := cState.Started.UnixNano()
		resp.Status.StartedAt = started
		finished := cState.Finished.UnixNano()
		resp.Status.FinishedAt = finished
		resp.Status.ExitCode = cState.ExitCode
	}

	resp.Status.State = rStatus

	logrus.Debugf("ContainerStatusResponse: %+v", resp)
	return resp, nil
}
