package server

import (
	"fmt"
	"time"

	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

// ContainerStats returns stats of the container. If the container does not
// exist, the call returns an error.
func (s *Server) ContainerStats(ctx context.Context, req *pb.ContainerStatsRequest) (resp *pb.ContainerStatsResponse, err error) {
	const operation = "container_stats"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()
	return nil, fmt.Errorf("not implemented")
}
