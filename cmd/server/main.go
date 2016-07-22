package main

import (
	"log"
	"net"
	"os"

	"github.com/kubernetes/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	"github.com/mrunalp/ocid/server"
	"google.golang.org/grpc"
)

const (
	unixDomainSocket = "/var/run/ocid.sock"
)

func main() {
	// Remove the socket if it already exists
	if _, err := os.Stat(unixDomainSocket); err == nil {
		if err := os.Remove(unixDomainSocket); err != nil {
			log.Fatal(err)
		}
	}
	lis, err := net.Listen("unix", unixDomainSocket)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	service, err := server.New("")
	if err != nil {
		log.Fatal(err)
	}
	runtime.RegisterRuntimeServiceServer(s, service)
	runtime.RegisterImageServiceServer(s, service)
	s.Serve(lis)
}
