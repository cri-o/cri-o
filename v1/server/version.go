package server

import (
	"github.com/cri-o/cri-o/internal/version"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	// kubeAPIVersion is the api version of kubernetes.
	// TODO: Track upstream code. For now it expects 0.1.0
	kubeAPIVersion = "0.1.0"
	// containerName is the name prepended in kubectl describe->Container ID:
	// cri-o://<CONTAINER_ID>
	containerName     = "cri-o"
	runtimeAPIVersion = "v1alpha1"
)

// Version returns the runtime name, runtime version and runtime API version
func (s *Server) Version(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
	return &pb.VersionResponse{
		Version:           kubeAPIVersion,
		RuntimeName:       containerName,
		RuntimeVersion:    version.Get().Version,
		RuntimeApiVersion: runtimeAPIVersion,
	}, nil
}
