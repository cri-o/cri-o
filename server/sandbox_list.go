package server

import (
	"context"

	"k8s.io/apimachinery/pkg/fields"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
)

// ListPodSandbox returns a list of SandBoxes.
func (s *Server) ListPodSandbox(ctx context.Context, req *types.ListPodSandboxRequest) (*types.ListPodSandboxResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	podList := s.filterSandboxList(ctx, req.Filter, s.ListSandboxes())
	respList := make([]*types.PodSandbox, 0, len(podList))

	for _, sb := range podList {
		// Skip sandboxes that aren't created yet
		if !sb.Created() {
			continue
		}

		pod := sb.CRISandbox()
		// Filter by other criteria such as state and labels.
		if filterSandbox(pod, req.Filter) {
			respList = append(respList, pod)
		}
	}

	return &types.ListPodSandboxResponse{
		Items: respList,
	}, nil
}

// filterSandboxList applies a protobuf-defined filter to retrieve only intended pod sandboxes. Not matching
// the filter is not considered an error but will return an empty response.
func (s *Server) filterSandboxList(ctx context.Context, filter *types.PodSandboxFilter, podList []*sandbox.Sandbox) []*sandbox.Sandbox {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	// Filter by pod id first.
	if filter == nil {
		return podList
	}

	if filter.Id != "" {
		id, err := s.ContainerServer.PodIDIndex().Get(filter.Id)
		if err != nil {
			// Not finding an ID in a filtered list should not be considered
			// and error; it might have been deleted when stop was done.
			// Log and return an empty struct.
			log.Warnf(ctx, "Unable to find pod %s with filter", filter.Id)

			return []*sandbox.Sandbox{}
		}

		sb := s.getSandbox(ctx, id)
		if sb == nil {
			podList = []*sandbox.Sandbox{}
		} else {
			podList = []*sandbox.Sandbox{sb}
		}
	}

	finalList := make([]*sandbox.Sandbox, 0, len(podList))

	for _, pod := range podList {
		// Skip sandboxes that aren't created yet
		if !pod.Created() {
			continue
		}

		if filter.State != nil {
			if pod.State() != filter.State.State {
				continue
			}
		}

		if filter.LabelSelector != nil {
			sel := fields.SelectorFromSet(filter.LabelSelector)
			if !sel.Matches(pod.Labels()) {
				continue
			}
		}

		finalList = append(finalList, pod)
	}

	return finalList
}

// filterSandbox returns whether passed container matches filtering criteria.
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
