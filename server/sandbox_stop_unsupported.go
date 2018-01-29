// +build !linux

package server

import (
	"fmt"

	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
)

func (s *Server) stopPodSandbox(ctx context.Context, req *pb.StopPodSandboxRequest) (resp *pb.StopPodSandboxResponse, err error) {
	return nil, fmt.Errorf("unsupported")
}
