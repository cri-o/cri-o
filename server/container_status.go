package server

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	json "github.com/json-iterator/go"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	oomKilledReason = "OOMKilled"
	completedReason = "Completed"
	errorReason     = "Error"
)

// ContainerStatus returns status of the container.
func (s *Server) ContainerStatus(ctx context.Context, req *types.ContainerStatusRequest) (*types.ContainerStatusResponse, error) {
	c, err := s.GetContainerFromShortID(req.ContainerId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerId, err)
	}

	containerID := c.ID()
	resp := &types.ContainerStatusResponse{
		Status: &types.ContainerStatus{
			Id:          containerID,
			Metadata:    c.Metadata(),
			Labels:      c.Labels(),
			Annotations: c.Annotations(),
			ImageRef:    c.ImageRef(),
			Image: &types.ImageSpec{
				Image: c.ImageName(),
			},
		},
	}

	mounts := []*types.Mount{}
	for _, cv := range c.Volumes() {
		mounts = append(mounts, &types.Mount{
			ContainerPath:  cv.ContainerPath,
			HostPath:       cv.HostPath,
			Readonly:       cv.Readonly,
			Propagation:    cv.Propagation,
			SelinuxRelabel: cv.SelinuxRelabel,
		})
	}
	resp.Status.Mounts = mounts

	cState := c.StateNoLock()
	rStatus := types.ContainerState_CONTAINER_UNKNOWN

	// If we defaulted to exit code not set earlier then we attempt to
	// get the exit code from the exit file again.
	if cState.Status == oci.ContainerStateStopped && cState.ExitCode == nil {
		err := s.Runtime().UpdateContainerStatus(ctx, c)
		if err != nil {
			log.Warnf(ctx, "Failed to UpdateStatus of container %s: %v", c.ID(), err)
		}
		cState = c.State()
	}

	resp.Status.CreatedAt = c.CreatedAt().UnixNano()
	switch cState.Status {
	case oci.ContainerStateCreated:
		rStatus = types.ContainerState_CONTAINER_CREATED
	case oci.ContainerStateRunning, oci.ContainerStatePaused:
		rStatus = types.ContainerState_CONTAINER_RUNNING
		started := cState.Started.UnixNano()
		resp.Status.StartedAt = started
	case oci.ContainerStateStopped:
		rStatus = types.ContainerState_CONTAINER_EXITED
		started := cState.Started.UnixNano()
		resp.Status.StartedAt = started
		finished := cState.Finished.UnixNano()
		resp.Status.FinishedAt = finished
		if cState.ExitCode == nil {
			resp.Status.ExitCode = -1
		} else {
			resp.Status.ExitCode = *cState.ExitCode
		}
		switch {
		case cState.OOMKilled:
			resp.Status.Reason = oomKilledReason
		case resp.Status.ExitCode == 0:
			resp.Status.Reason = completedReason
		default:
			resp.Status.Reason = errorReason
			resp.Status.Message = cState.Error
		}
	}

	resp.Status.State = rStatus
	resp.Status.LogPath = c.LogPath()

	if req.Verbose {
		info, err := s.createContainerInfo(c)
		if err != nil {
			return nil, fmt.Errorf("creating container info: %w", err)
		}
		resp.Info = info
	}

	return resp, nil
}

func (s *Server) createContainerInfo(container *oci.Container) (map[string]string, error) {
	metadata, err := s.StorageRuntimeServer().GetContainerMetadata(container.ID())
	if err != nil {
		return nil, fmt.Errorf("getting container metadata: %w", err)
	}

	info := struct {
		SandboxID   string    `json:"sandboxID"`
		Pid         int       `json:"pid"`
		RuntimeSpec spec.Spec `json:"runtimeSpec"`
		Privileged  bool      `json:"privileged"`
	}{
		container.Sandbox(),
		container.State().Pid,
		container.Spec(),
		metadata.Privileged,
	}
	bytes, err := json.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %v: %w", info, err)
	}
	return map[string]string{"info": string(bytes)}, nil
}
