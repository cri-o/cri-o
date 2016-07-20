package main

import (
	"log"
	"net"
	"os"

	"github.com/kubernetes/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	"github.com/mrunalp/ocid/server"
	"github.com/urfave/cli"
	"google.golang.org/grpc"
)

const (
	unixDomainSocket = "/var/run/ocid.sock"
)

func main() {
	app := cli.NewApp()
	app.Name = "ocic"
	app.Usage = "client for ocid"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "sandboxdir",
			Value: "/var/lib/ocid/sandbox",
			Usage: "ocid pod sandbox dir",
		},
	}

	app.Action = func(c *cli.Context) error {
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

		sandboxDir := c.String("sandboxdir")
		service, err := server.New("", sandboxDir)
		if err != nil {
			log.Fatal(err)
		}

		runtime.RegisterRuntimeServiceServer(s, service)
		runtime.RegisterImageServiceServer(s, service)
		s.Serve(lis)
		return nil
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
