// +build !linux

package server

import (
	"fmt"

	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *Server) runPodSandbox(ctx context.Context, req *pb.RunPodSandboxRequest) (resp *pb.RunPodSandboxResponse, err error) {
	return nil, fmt.Errorf("unsupported")
}
