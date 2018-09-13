package server

import (
	"time"

	"github.com/containers/image/types"
	"github.com/kubernetes-sigs/cri-o/oci"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

const (
	oomKilledReason = "OOMKilled"
	completedReason = "Completed"
	errorReason     = "Error"
)

// ContainerStatus returns status of the container.
func (s *Server) ContainerStatus(ctx context.Context, req *pb.ContainerStatusRequest) (resp *pb.ContainerStatusResponse, err error) {
	const operation = "container_status"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()
	logrus.Debugf("ContainerStatusRequest %+v", req)
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, err
	}

	containerID := c.ID()
	resp = &pb.ContainerStatusResponse{
		Status: &pb.ContainerStatus{
			Id:          containerID,
			Metadata:    c.Metadata(),
			Labels:      c.Labels(),
			Annotations: c.Annotations(),
			ImageRef:    c.ImageRef(),
		},
	}
	resp.Status.Image = &pb.ImageSpec{Image: c.Image()}
	if status, err := s.StorageImageServer().ImageStatus(&types.SystemContext{}, c.ImageRef()); err == nil {
		resp.Status.Image.Image = status.Name
	}

	mounts := []*pb.Mount{}
	for _, cv := range c.Volumes() {
		mounts = append(mounts, &pb.Mount{
			ContainerPath: cv.ContainerPath,
			HostPath:      cv.HostPath,
			Readonly:      cv.Readonly,
		})
	}
	resp.Status.Mounts = mounts

	cState := s.Runtime().ContainerStatus(c)
	rStatus := pb.ContainerState_CONTAINER_UNKNOWN

	// If we defaulted to exit code -1 earlier then we attempt to
	// get the exit code from the exit file again.
	if cState.ExitCode == -1 {
		err := s.Runtime().UpdateStatus(c)
		if err != nil {
			logrus.Warnf("Failed to UpdateStatus of container %s: %v", c.ID(), err)
		}
		cState = s.Runtime().ContainerStatus(c)
	}

	created := c.CreatedAt().UnixNano()
	switch cState.Status {
	case oci.ContainerStateCreated:
		rStatus = pb.ContainerState_CONTAINER_CREATED
		resp.Status.CreatedAt = created
	case oci.ContainerStateRunning:
		rStatus = pb.ContainerState_CONTAINER_RUNNING
		resp.Status.CreatedAt = created
		started := cState.Started.UnixNano()
		resp.Status.StartedAt = started
	case oci.ContainerStateStopped:
		rStatus = pb.ContainerState_CONTAINER_EXITED
		resp.Status.CreatedAt = created
		started := cState.Started.UnixNano()
		resp.Status.StartedAt = started
		finished := cState.Finished.UnixNano()
		resp.Status.FinishedAt = finished
		resp.Status.ExitCode = cState.ExitCode
		switch {
		case cState.OOMKilled:
			resp.Status.Reason = oomKilledReason
		case cState.ExitCode == 0:
			resp.Status.Reason = completedReason
		default:
			resp.Status.Reason = errorReason
			resp.Status.Message = cState.Error
		}
	}

	resp.Status.State = rStatus
	resp.Status.LogPath = c.LogPath()

	logrus.Debugf("ContainerStatusResponse: %+v", resp)
	return resp, nil
}
