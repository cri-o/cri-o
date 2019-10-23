package server

import (
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/pkg/log"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/fields"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
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
func (s *Server) ListPodSandbox(ctx context.Context, req *pb.ListPodSandboxRequest) (resp *pb.ListPodSandboxResponse, err error) {
	var pods []*pb.PodSandbox
	var podList []*sandbox.Sandbox
	podList = append(podList, s.ContainerServer.ListSandboxes()...)

	filter := req.Filter
	// Filter by pod id first.
	if filter != nil {
		if filter.Id != "" {
			id, err := s.PodIDIndex().Get(filter.Id)
			if err != nil {
				// Not finding an ID in a filtered list should not be considered
				// and error; it might have been deleted when stop was done.
				// Log and return an empty struct.
				log.Warnf(ctx, "unable to find pod %s with filter", filter.Id)
				return &pb.ListPodSandboxResponse{}, nil
			}
			sb := s.getSandbox(id)
			if sb == nil {
				podList = []*sandbox.Sandbox{}
			} else {
				podList = []*sandbox.Sandbox{sb}
			}
		}
	}

	for _, sb := range podList {
		// Skip sandboxes that aren't created yet
		if !sb.Created() {
			continue
		}
		podInfraContainer := sb.InfraContainer()
		if podInfraContainer == nil {
			// this can't really happen, but if it does because of a bug
			// it's better not to panic
			continue
		}
		cState := podInfraContainer.StateNoLock()
		rStatus := pb.PodSandboxState_SANDBOX_NOTREADY
		if cState.Status == oci.ContainerStateRunning {
			rStatus = pb.PodSandboxState_SANDBOX_READY
		}

		pod := &pb.PodSandbox{
			Id:          sb.ID(),
			CreatedAt:   podInfraContainer.CreatedAt().UnixNano(),
			State:       rStatus,
			Labels:      sb.Labels(),
			Annotations: sb.Annotations(),
			Metadata:    sb.Metadata(),
		}

		// Filter by other criteria such as state and labels.
		if filterSandbox(pod, req.Filter) {
			pods = append(pods, pod)
		}
	}

	resp = &pb.ListPodSandboxResponse{
		Items: pods,
	}
	return resp, nil
}
