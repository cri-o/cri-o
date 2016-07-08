package main

import (
	"log"
	"net"

	"github.com/kubernetes/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	"github.com/mrunalp/ocid/server"
	"google.golang.org/grpc"
)

const (
	port = ":49999"
)

func main() {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	runtime.RegisterRuntimeServiceServer(s, &server.Server{})
	s.Serve(lis)
}
