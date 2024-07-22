package server

import (
	"fmt"
	"time"

	json "github.com/json-iterator/go"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
)

const (
	oomKilledReason     = "OOMKilled"
	seccompKilledReason = "seccomp killed"
	completedReason     = "Completed"
	errorReason         = "Error"
)

// ContainerStatus returns status of the container.
func (s *Server) ContainerStatus(ctx context.Context, req *types.ContainerStatusRequest) (*types.ContainerStatusResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	c, err := s.GetContainerFromShortID(ctx, req.ContainerId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "could not find container %q: %v", req.ContainerId, err)
	}

	containerID := c.ID()
	imageRef := c.CRIContainer().ImageRef
	imageNameInSpec := ""
	if imageName := c.ImageName(); imageName != nil {
		imageNameInSpec = imageName.StringForOutOfProcessConsumptionOnly()
	}
	imageID := ""
	if c.ImageID() != nil {
		imageID = c.ImageID().IDStringForOutOfProcessConsumptionOnly()
	}
	resp := &types.ContainerStatusResponse{
		Status: &types.ContainerStatus{
			Id:          containerID,
			Metadata:    c.Metadata(),
			Labels:      c.Labels(),
			Annotations: c.Annotations(),
			ImageId:     imageID,
			ImageRef:    imageRef,
			Image: &types.ImageSpec{
				Image: imageNameInSpec,
			},
			User: c.RuntimeUser(),
		},
	}

	mounts := []*types.Mount{}
	for _, cv := range c.Volumes() {
		mounts = append(mounts, &types.Mount{
			ContainerPath:     cv.ContainerPath,
			HostPath:          cv.HostPath,
			Readonly:          cv.Readonly,
			RecursiveReadOnly: cv.RecursiveReadOnly,
			Propagation:       cv.Propagation,
			SelinuxRelabel:    cv.SelinuxRelabel,
			Image:             cv.Image,
		})
	}
	resp.Status.Mounts = mounts

	containerSpec := c.Spec()
	if containerSpec.Linux != nil {
		resp.Status.Resources = c.GetResources()
	}

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
		case cState.SeccompKilled:
			resp.Status.Reason = seccompKilledReason
			resp.Status.Message = cState.Error
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

type containerInfo struct {
	SandboxID   string    `json:"sandboxID"`
	Pid         int       `json:"pid"`
	RuntimeSpec spec.Spec `json:"runtimeSpec"`
	Privileged  bool      `json:"privileged"`
}

type containerInfoCheckpointRestore struct {
	CheckpointedAt time.Time `json:"checkpointedAt"`
	Restored       bool      `json:"restored"`
}

func (s *Server) createContainerInfo(container *oci.Container) (map[string]string, error) {
	metadata, err := s.StorageRuntimeServer().GetContainerMetadata(container.ID())
	if err != nil {
		return nil, fmt.Errorf("getting container metadata: %w", err)
	}

	bytes, err := func(metadata *storage.RuntimeContainerMetadata) ([]byte, error) {
		localContainerInfo := containerInfo{
			SandboxID:   container.Sandbox(),
			Pid:         container.StateNoLock().InitPid,
			RuntimeSpec: container.Spec(),
			Privileged:  metadata.Privileged,
		}

		if s.config.CheckpointRestore() {
			localContainerInfoCheckpointRestore := containerInfoCheckpointRestore{
				CheckpointedAt: container.CheckpointedAt(),
				Restored:       container.Restore(),
			}
			info := struct {
				containerInfo
				containerInfoCheckpointRestore
			}{
				localContainerInfo,
				localContainerInfoCheckpointRestore,
			}
			return json.Marshal(info)
		}

		return json.Marshal(localContainerInfo)
	}(&metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %w", err)
	}
	return map[string]string{"info": string(bytes)}, nil
}
