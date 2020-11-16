// +build !linux

package server

import (
	"fmt"

	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *Server) runPodSandbox(ctx context.Context, req *pb.RunPodSandboxRequest) (*pb.RunPodSandboxResponse, error) {
	return nil, fmt.Errorf("unsupported")
}
