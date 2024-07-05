package server

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/fields"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
)

// filterContainer returns whether passed container matches filtering criteria.
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
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	// Filter using container id and pod id first.
	if filter.Id != "" {
		c, err := s.ContainerServer.GetContainerFromShortID(ctx, filter.Id)
		if err != nil {
			// If we don't find a container ID with a filter, it should not
			// be considered an error.  Log a warning and return an empty struct
			log.Warnf(ctx, "Unable to find container ID %s", filter.Id)
			return nil
		}
		switch {
		case filter.PodSandboxId == "":
			return []*oci.Container{c}
		case strings.HasPrefix(c.Sandbox(), filter.PodSandboxId):
			return []*oci.Container{c}
		default:
			return nil
		}
	} else if filter.PodSandboxId != "" {
		sb, err := s.getPodSandboxFromRequest(ctx, filter.PodSandboxId)
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
	ctx, span := log.StartSpan(ctx)
	defer span.End()
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
		c := ctr.CRIContainer()
		// Filter by other criteria such as state and labels.
		if filterContainer(c, req.Filter) {
			ctrs = append(ctrs, c)
		}
	}

	return &types.ListContainersResponse{
		Containers: ctrs,
	}, nil
}
