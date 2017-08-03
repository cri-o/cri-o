package server

import (
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// CreateContainer creates a new container in the specified PodSandbox
func (s *Server) CreateContainer(ctx context.Context, req *pb.CreateContainerRequest) (res *pb.CreateContainerResponse, err error) {
	ctr, err := s.ContainerCreate(ctx, req)
	if err != nil {
		return nil, err
	}

	resp := &pb.CreateContainerResponse{
		ContainerId: ctr.ID(),
	}

	logrus.Debugf("CreateContainerResponse: %+v", resp)
	return resp, nil
}
