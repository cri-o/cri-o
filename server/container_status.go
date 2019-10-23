package server

import (
	"strconv"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/pkg/log"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	oomKilledReason = "OOMKilled"
	completedReason = "Completed"
	errorReason     = "Error"
)

// ContainerStatus returns status of the container.
func (s *Server) ContainerStatus(ctx context.Context, req *pb.ContainerStatusRequest) (resp *pb.ContainerStatusResponse, err error) {
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
			Image: &pb.ImageSpec{
				Image: c.ImageName(),
			},
		},
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

	cState := c.State()
	rStatus := pb.ContainerState_CONTAINER_UNKNOWN

	// If we defaulted to exit code -1 earlier then we attempt to
	// get the exit code from the exit file again.
	if cState.ExitCode == -1 {
		err := s.Runtime().UpdateContainerStatus(c)
		if err != nil {
			log.Warnf(ctx, "Failed to UpdateStatus of container %s: %v", c.ID(), err)
		}
		cState = c.State()
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

	if req.Verbose {
		resp.Info = map[string]string{
			"pid":       strconv.Itoa(c.State().Pid),
			"sandboxId": c.Sandbox(),
		}
	}

	return resp, nil
}
