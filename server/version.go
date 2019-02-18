package server

import (
	"time"

	"github.com/kubernetes-sigs/cri-o/version"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
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
func (s *Server) Version(ctx context.Context, req *pb.VersionRequest) (resp *pb.VersionResponse, err error) {
	const operation = "version"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	return &pb.VersionResponse{
		Version:           kubeAPIVersion,
		RuntimeName:       containerName,
		RuntimeVersion:    version.Version,
		RuntimeApiVersion: runtimeAPIVersion,
	}, nil
}
