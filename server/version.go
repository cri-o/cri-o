package server

import (
	"context"

	"github.com/cri-o/cri-o/internal/version"
	"github.com/cri-o/cri-o/server/cri/types"
)

const (
	// kubeAPIVersion is the api version of kubernetes.
	// TODO: Track upstream code. For now it expects 0.1.0
	kubeAPIVersion = "0.1.0"
	// containerName is the name prepended in kubectl describe->Container ID:
	// cri-o://<CONTAINER_ID>
	containerName = "cri-o"
)

// Version returns the runtime name, runtime version and runtime API version
func (s *Server) Version(_ context.Context, apiVersion string) (*types.VersionResponse, error) {
	return &types.VersionResponse{
		Version:           kubeAPIVersion,
		RuntimeName:       containerName,
		RuntimeVersion:    version.Get().Version,
		RuntimeAPIVersion: apiVersion,
	}, nil
}
