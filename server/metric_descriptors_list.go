package server

import (
	"context"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	statsserver "github.com/cri-o/cri-o/internal/lib/stats"
)

// ListMetricDescriptors lists all metric descriptors.
func (s *Server) ListMetricDescriptors(ctx context.Context, req *types.ListMetricDescriptorsRequest) (*types.ListMetricDescriptorsResponse, error) {
	includedKeys := s.config.IncludedPodMetrics
	all := len(includedKeys) > 1 && includedKeys[0] == statsserver.AllMetrics
	// It's guaranteed in config validation that if all is specified, it's the only one element in the slice.
	descriptorsMap := s.PopulateMetricDescriptors(includedKeys, all)

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
