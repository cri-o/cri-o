package manager

import (
	"github.com/kubernetes-incubator/cri-o/oci"
	"k8s.io/kubernetes/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// filterSandbox returns whether passed container matches filtering criteria
func filterSandbox(p *pb.PodSandbox, filter *pb.PodSandboxFilter) bool {
	if filter != nil {
		if filter.State != nil {
			if *p.State != *filter.State {
				return false
			}
		}
		if filter.LabelSelector != nil {
			sel := fields.SelectorFromSet(filter.LabelSelector)
			if !sel.Matches(fields.Set(p.Labels)) {
				return false
			}
		}
	}
	return true
}

// ListPodSandbox returns a list of SandBoxes.
func (m *Manager) ListPodSandbox(filter *pb.PodSandboxFilter) ([]*pb.PodSandbox, error) {
	var pods []*pb.PodSandbox
	var podList []*sandbox
	for _, sb := range m.state.sandboxes {
		podList = append(podList, sb)
	}

	// Filter by pod id first.
	if filter != nil {
		if filter.Id != nil {
			id, err := m.podIDIndex.Get(*filter.Id)
			if err != nil {
				return nil, err
			}
			sb := m.getSandbox(id)
			if sb == nil {
				podList = []*sandbox{}
			} else {
				podList = []*sandbox{sb}
			}
		}
	}

	for _, sb := range podList {
		podInfraContainer := sb.infraContainer
		if podInfraContainer == nil {
			// this can't really happen, but if it does because of a bug
			// it's better not to panic
			continue
		}
		if err := m.runtime.UpdateStatus(podInfraContainer); err != nil {
			return nil, err
		}
		cState := m.runtime.ContainerStatus(podInfraContainer)
		created := cState.Created.UnixNano()
		rStatus := pb.PodSandboxState_SANDBOX_NOTREADY
		if cState.Status == oci.ContainerStateRunning {
			rStatus = pb.PodSandboxState_SANDBOX_READY
		}

		pod := &pb.PodSandbox{
			Id:          &sb.id,
			CreatedAt:   int64Ptr(created),
			State:       &rStatus,
			Labels:      sb.labels,
			Annotations: sb.annotations,
			Metadata:    sb.metadata,
		}

		// Filter by other criteria such as state and labels.
		if filterSandbox(pod, filter) {
			pods = append(pods, pod)
		}
	}

	return pods, nil
}
