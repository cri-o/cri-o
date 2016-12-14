package server

import (
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// Version returns the runtime name, runtime version and runtime API version
func (s *Server) Version(ctx context.Context, req *pb.VersionRequest) (*pb.VersionResponse, error) {

	versionResp, err := s.manager.Version()
	if err != nil {
		return nil, err
	}

	// TODO: Track upstream code. For now it expects 0.1.0
	version := "0.1.0"

	// taking const address
	rav := runtimeAPIVersion

	return &pb.VersionResponse{
		Version:           &version,
		RuntimeName:       &versionResp.RuntimeName,
		RuntimeVersion:    &versionResp.RuntimeVersion,
		RuntimeApiVersion: &rav,
	}, nil
}
