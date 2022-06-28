package server

import (
	"context"

	"github.com/cri-o/cri-o/internal/version"
	"github.com/pkg/errors"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
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
	info, err := version.Get(false)
	if err != nil {
		return nil, errors.Wrap(err, "get server version")
	}
	return &types.VersionResponse{
		Version:           kubeAPIVersion,
		RuntimeName:       containerName,
		RuntimeVersion:    info.Version,
		RuntimeApiVersion: apiVersion,
	}, nil
}
