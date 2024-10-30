package server

import (
	"context"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ListMetricDescriptors lists all metric descriptors.
func (s *Server) ListMetricDescriptors(ctx context.Context, req *types.ListMetricDescriptorsRequest) (*types.ListMetricDescriptorsResponse, error) {
	includedKeys := s.config.IncludedPodMetrics
	descriptorsMap := s.StatsServer.PopulateMetricDescriptors(includedKeys)

	// Flatten the map of descriptors to a slice.
	var flattenedDescriptors []*types.MetricDescriptor
	for _, descriptors := range descriptorsMap {
		flattenedDescriptors = append(flattenedDescriptors, descriptors...)
	}

	response := &types.ListMetricDescriptorsResponse{
		Descriptors: flattenedDescriptors,
	}
	return response, nil
}
