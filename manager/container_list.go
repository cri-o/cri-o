package manager

import (
	"github.com/kubernetes-incubator/cri-o/oci"
	"k8s.io/kubernetes/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// filterContainer returns whether passed container matches filtering criteria
func filterContainer(c *pb.Container, filter *pb.ContainerFilter) bool {
	if filter != nil {
		if filter.State != nil {
			if *c.State != *filter.State {
				return false
			}
		}
		if filter.LabelSelector != nil {
			sel := fields.SelectorFromSet(filter.LabelSelector)
			if !sel.Matches(fields.Set(c.Labels)) {
				return false
			}
		}
	}
	return true
}

// ListContainers lists all containers by filters.
func (m *Manager) ListContainers(filter *pb.ContainerFilter) ([]*pb.Container, error) {
	var ctrs []*pb.Container
	ctrList := m.state.containers.List()

	// Filter using container id and pod id first.
	if filter != nil {
		if filter.Id != nil {
			id, err := m.ctrIDIndex.Get(*filter.Id)
			if err != nil {
				return nil, err
			}
			c := m.state.containers.Get(id)
			if c != nil {
				if filter.PodSandboxId != nil {
					if c.Sandbox() == *filter.PodSandboxId {
						ctrList = []*oci.Container{c}
					} else {
						ctrList = []*oci.Container{}
					}

				} else {
					ctrList = []*oci.Container{c}
				}
			}
		} else {
			if filter.PodSandboxId != nil {
				pod := m.state.sandboxes[*filter.PodSandboxId]
				if pod == nil {
					ctrList = []*oci.Container{}
				} else {
					ctrList = pod.containers.List()
				}
			}
		}
	}

	for _, ctr := range ctrList {
		if err := m.runtime.UpdateStatus(ctr); err != nil {
			return nil, err
		}

		podSandboxID := ctr.Sandbox()
		cState := m.runtime.ContainerStatus(ctr)
		created := cState.Created.UnixNano()
		rState := pb.ContainerState_CONTAINER_UNKNOWN
		cID := ctr.ID()

		c := &pb.Container{
			Id:           &cID,
			PodSandboxId: &podSandboxID,
			CreatedAt:    int64Ptr(created),
			Labels:       ctr.Labels(),
			Metadata:     ctr.Metadata(),
			Annotations:  ctr.Annotations(),
			Image:        ctr.Image(),
		}

		switch cState.Status {
		case oci.ContainerStateCreated:
			rState = pb.ContainerState_CONTAINER_CREATED
		case oci.ContainerStateRunning:
			rState = pb.ContainerState_CONTAINER_RUNNING
		case oci.ContainerStateStopped:
			rState = pb.ContainerState_CONTAINER_EXITED
		}
		c.State = &rState

		// Filter by other criteria such as state and labels.
		if filterContainer(c, filter) {
			ctrs = append(ctrs, c)
		}
	}

	return ctrs, nil
}
