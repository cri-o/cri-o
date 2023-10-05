package server

import (
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ListMetricDescriptors lists all metric descriptors.
func (s *Server) ListMetricDescriptors(ctx context.Context, req *types.ListMetricDescriptorsRequest) (*types.ListMetricDescriptorsResponse, error) {
	includedKeys := []string{"cpu", "memory", "network"} // You can modify this as needed
	descriptorsMap := s.StatsServer.PopulateMetricDescriptors(includedKeys)

	// Flatten the map of descriptors to a slice
	var flattenedDescriptors []*types.MetricDescriptor
	for _, descriptors := range descriptorsMap {
		flattenedDescriptors = append(flattenedDescriptors, descriptors...)
	}

	// Create the response with the flattened list of descriptors
	response := &types.ListMetricDescriptorsResponse{
		Descriptors: flattenedDescriptors,
	}
	return response, nil
}
