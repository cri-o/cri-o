package server

import (
	"fmt"
	"time"

	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

// ListContainerStats returns stats of all running containers.
func (s *Server) ListContainerStats(ctx context.Context, req *pb.ListContainerStatsRequest) (resp *pb.ListContainerStatsResponse, err error) {
	const operation = "list_container_stats"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()
	return nil, fmt.Errorf("not implemented")
}
