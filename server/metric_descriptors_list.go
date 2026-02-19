package server

import (
	"context"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ListMetricDescriptors lists all metric descriptors.
func (s *Server) ListMetricDescriptors(ctx context.Context, req *types.ListMetricDescriptorsRequest) (*types.ListMetricDescriptorsResponse, error) {
	includedKeys := s.config.IncludedPodMetrics
	descriptorsMap := s.PopulateMetricDescriptors(includedKeys)

	// Flatten the map of descriptors to a slice.
	totalDescriptors := 0
	for _, descriptors := range descriptorsMap {
		totalDescriptors += len(descriptors)
	}

	flattenedDescriptors := make([]*types.MetricDescriptor, 0, totalDescriptors)
	for _, descriptors := range descriptorsMap {
		flattenedDescriptors = append(flattenedDescriptors, descriptors...)
	}

	response := &types.ListMetricDescriptorsResponse{
		Descriptors: flattenedDescriptors,
	}

	return response, nil
}
