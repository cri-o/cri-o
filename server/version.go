package server

import (
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	"golang.org/x/net/context"
)

// Version returns the runtime name, runtime version and runtime API version
func (s *Server) Version(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {
	version, err := getGPRCVersion()
	if err != nil {
		return nil, err
	}

	runtimeVersion, err := s.runtime.Version()
	if err != nil {
		return nil, err
	}

	// taking const address
	rav := runtimeAPIVersion
	runtimeName := s.runtime.Name()

	return &pb.VersionResponse{
		Version:           &version,
		RuntimeName:       &runtimeName,
		RuntimeVersion:    &runtimeVersion,
		RuntimeApiVersion: &rav,
	}, nil
}
