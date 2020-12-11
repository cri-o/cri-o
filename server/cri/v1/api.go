package v1

import (
	"google.golang.org/grpc"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/server"
)

type Service interface {
	pb.RuntimeServiceServer
	pb.ImageServiceServer
}

type service struct {
	server *server.Server
}

// Register registers the runtime and image service with the provided grpc server
func Register(grpcServer *grpc.Server, crioServer *server.Server) {
	s := &service{crioServer}
	pb.RegisterRuntimeServiceServer(grpcServer, s)
	pb.RegisterImageServiceServer(grpcServer, s)
}
