package server

import (
	"context"
	"strings"

	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/server/cri/types"
	"k8s.io/apimachinery/pkg/fields"
)

// filterContainer returns whether passed container matches filtering criteria
func filterContainer(c *types.Container, filter *types.ContainerFilter) bool {
	if filter != nil {
		if filter.State != nil {
			if c.State != filter.State.State {
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

// filterContainerList applies a protobuf-defined filter to retrieve only intended containers. Not matching
// the filter is not considered an error but will return an empty response.
func (s *Server) filterContainerList(ctx context.Context, filter *types.ContainerFilter, origCtrList []*oci.Container) []*oci.Container {
	// Filter using container id and pod id first.
	if filter.ID != "" {
		c, err := s.ContainerServer.GetContainerFromShortID(filter.ID)
		if err != nil {
			// If we don't find a container ID with a filter, it should not
			// be considered an error.  Log a warning and return an empty struct
			log.Warnf(ctx, "Unable to find container ID %s", filter.ID)
			return nil
		}
		switch {
		case filter.PodSandboxID == "":
			return []*oci.Container{c}
		case strings.HasPrefix(c.Sandbox(), filter.PodSandboxID):
			return []*oci.Container{c}
		default:
			return nil
		}
	} else if filter.PodSandboxID != "" {
		sb, err := s.getPodSandboxFromRequest(filter.PodSandboxID)
		if err != nil {
			return nil
		}
		return sb.Containers().List()
	}
	log.Debugf(ctx, "No filters were applied, returning full container list")
	return origCtrList
}

// ListContainers lists all containers by filters.
func (s *Server) ListContainers(ctx context.Context, req *types.ListContainersRequest) (*types.ListContainersResponse, error) {
	var ctrs []*types.Container
	filter := req.Filter
	ctrList, err := s.ContainerServer.ListContainers()
	if err != nil {
		return nil, err
	}

	if filter != nil {
		ctrList = s.filterContainerList(ctx, filter, ctrList)
	}

	for _, ctr := range ctrList {
		// Skip over containers that are still being created
		if !ctr.Created() {
			continue
		}
		podSandboxID := ctr.Sandbox()
		cState := ctr.StateNoLock()
		created := ctr.CreatedAt().UnixNano()
		rState := types.ContainerStateContainerUnknown
		cID := ctr.ID()
		img := &types.ImageSpec{
			Image: ctr.Image(),
		}
		c := &types.Container{
			ID:           cID,
			PodSandboxID: podSandboxID,
			CreatedAt:    created,
			Labels:       ctr.Labels(),
			Metadata: &types.ContainerMetadata{
				Name:    ctr.Metadata().Name,
				Attempt: ctr.Metadata().Attempt,
			},
			Annotations: ctr.Annotations(),
			Image:       img,
			ImageRef:    ctr.ImageRef(),
		}

		switch cState.Status {
		case oci.ContainerStateCreated:
			rState = types.ContainerStateContainerCreated
		case oci.ContainerStateRunning:
			rState = types.ContainerStateContainerRunning
		case oci.ContainerStateStopped:
			rState = types.ContainerStateContainerExited
		}
		c.State = rState

		// Filter by other criteria such as state and labels.
		if filterContainer(c, req.Filter) {
			ctrs = append(ctrs, c)
		}
	}

	return &types.ListContainersResponse{
		Containers: ctrs,
	}, nil
}
