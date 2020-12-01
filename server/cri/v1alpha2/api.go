package v1alpha2

import (
	"google.golang.org/grpc"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

type Service interface {
	pb.RuntimeServiceServer
	pb.ImageServiceServer
}

type service struct{}

// New creates a new v1alpha2 Service instance.
func New(server *grpc.Server) Service {
	s := &service{}
	pb.RegisterRuntimeServiceServer(server, s)
	pb.RegisterImageServiceServer(server, s)
	return s
}
