package server

import (
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// filterSandbox returns whether passed container matches filtering criteria
func filterSandbox(p *pb.PodSandbox, filter *pb.PodSandboxFilter) bool {
	if filter != nil {
		if filter.State != nil {
			if p.State != filter.State.State {
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
func (s *Server) ListPodSandbox(ctx context.Context, req *pb.ListPodSandboxRequest) (*pb.ListPodSandboxResponse, error) {
	logrus.Debugf("ListPodSandboxRequest %+v", req)
	var pods []*pb.PodSandbox
	var podList []*sandbox
	for _, sb := range s.state.sandboxes {
		podList = append(podList, sb)
	}

	filter := req.Filter
	// Filter by pod id first.
	if filter != nil {
		if filter.Id != "" {
			id, err := s.podIDIndex.Get(filter.Id)
			if err != nil {
				return nil, err
			}
			sb := s.getSandbox(id)
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
		if err := s.runtime.UpdateStatus(podInfraContainer); err != nil {
			logrus.Warnf("error updating status for pod %s: %v", sb.id, err)
		}
		cState := s.runtime.ContainerStatus(podInfraContainer)
		// TODO: we must be saving pod creation somewhere on disk for when we restore
		created := time.Time{}.UnixNano()
		rStatus := pb.PodSandboxState_SANDBOX_NOTREADY
		if cState != nil {
			created = cState.Created.UnixNano()
			if cState.Status == oci.ContainerStateRunning {
				rStatus = pb.PodSandboxState_SANDBOX_READY
			}
		}

		pod := &pb.PodSandbox{
			Id:          sb.id,
			CreatedAt:   created,
			State:       rStatus,
			Labels:      sb.labels,
			Annotations: sb.annotations,
			Metadata:    sb.metadata,
		}

		// Filter by other criteria such as state and labels.
		if filterSandbox(pod, req.Filter) {
			pods = append(pods, pod)
		}
	}

	resp := &pb.ListPodSandboxResponse{
		Items: pods,
	}
	logrus.Debugf("ListPodSandboxResponse %+v", resp)
	return resp, nil
}
