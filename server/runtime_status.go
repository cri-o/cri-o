package server

import (
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// Status returns the status of the runtime
func (s *Server) Status(ctx context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	status, err := s.manager.Status()
	if err != nil {
		return nil, err
	}

	resp := &pb.StatusResponse{
		Status: status,
	}

	return resp, nil
}
