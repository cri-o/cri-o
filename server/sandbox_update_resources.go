package server

import (
	"context"
	"errors"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// UpdatePodSandboxResources updates Config of the pod sandbox.
func (s *Server) UpdatePodSandboxResources(ctx context.Context, req *types.UpdatePodSandboxResourcesRequest) (*types.UpdatePodSandboxResourcesResponse, error) {
	// TODO: implement this function
	// https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/1287-in-place-update-pod-resources/README.md#cri-changes
	return nil, errors.New("not implemented yet")
}
