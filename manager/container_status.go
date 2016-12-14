package manager

import (
	"github.com/kubernetes-incubator/cri-o/oci"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// ContainerStatus returns status of the container.
func (m *Manager) ContainerStatus(ctrID string) (*pb.ContainerStatus, error) {
	c, err := m.getContainerWithPartialID(ctrID)
	if err != nil {
		return nil, err
	}

	if err := m.runtime.UpdateStatus(c); err != nil {
		return nil, err
	}

	containerID := c.ID()
	status := &pb.ContainerStatus{
		Id:       &containerID,
		Metadata: c.Metadata(),
	}

	cState := m.runtime.ContainerStatus(c)
	rStatus := pb.ContainerState_CONTAINER_UNKNOWN

	switch cState.Status {
	case oci.ContainerStateCreated:
		rStatus = pb.ContainerState_CONTAINER_CREATED
		created := cState.Created.UnixNano()
		status.CreatedAt = int64Ptr(created)
	case oci.ContainerStateRunning:
		rStatus = pb.ContainerState_CONTAINER_RUNNING
		created := cState.Created.UnixNano()
		status.CreatedAt = int64Ptr(created)
		started := cState.Started.UnixNano()
		status.StartedAt = int64Ptr(started)
	case oci.ContainerStateStopped:
		rStatus = pb.ContainerState_CONTAINER_EXITED
		created := cState.Created.UnixNano()
		status.CreatedAt = int64Ptr(created)
		started := cState.Started.UnixNano()
		status.StartedAt = int64Ptr(started)
		finished := cState.Finished.UnixNano()
		status.FinishedAt = int64Ptr(finished)
		status.ExitCode = int32Ptr(cState.ExitCode)
	}

	status.State = &rStatus

	return status, nil
}
