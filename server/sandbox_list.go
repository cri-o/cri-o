package server

import (
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/server/cri/types"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/fields"
)

// filterSandbox returns whether passed container matches filtering criteria
func filterSandbox(p *types.PodSandbox, filter *types.PodSandboxFilter) bool {
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
func (s *Server) ListPodSandbox(ctx context.Context, req *types.ListPodSandboxRequest) (*types.ListPodSandboxResponse, error) {
	var pods []*types.PodSandbox
	var podList []*sandbox.Sandbox
	podList = append(podList, s.ContainerServer.ListSandboxes()...)

	filter := req.Filter
	// Filter by pod id first.
	if filter != nil {
		if filter.ID != "" {
			id, err := s.PodIDIndex().Get(filter.ID)
			if err != nil {
				// Not finding an ID in a filtered list should not be considered
				// and error; it might have been deleted when stop was done.
				// Log and return an empty struct.
				log.Warnf(ctx, "Unable to find pod %s with filter", filter.ID)
				return &types.ListPodSandboxResponse{}, nil
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

		rStatus := types.PodSandboxStateSandboxNotReady
		if sb.Ready(false) {
			rStatus = types.PodSandboxStateSandboxReady
		}

		pod := &types.PodSandbox{
			ID:          sb.ID(),
			CreatedAt:   sb.CreatedAt().UnixNano(),
			State:       rStatus,
			Labels:      sb.Labels(),
			Annotations: sb.Annotations(),
			Metadata: &types.PodSandboxMetadata{
				Name:      sb.Metadata().Name,
				UID:       sb.Metadata().UID,
				Namespace: sb.Metadata().Namespace,
				Attempt:   sb.Metadata().Attempt,
			},
		}

		// Filter by other criteria such as state and labels.
		if filterSandbox(pod, req.Filter) {
			pods = append(pods, pod)
		}
	}

	return &types.ListPodSandboxResponse{
		Items: pods,
	}, nil
}
