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
			Value: "/var/lib/ocid/sandboxes",
			Usage: "ocid pod sandbox dir",
		},
		cli.StringFlag{
			Name:  "runtime",
			Value: "/usr/bin/runc",
			Usage: "OCI runtime path",
		},
		cli.StringFlag{
			Name:  "containerdir",
			Value: "/var/lib/ocid/containers",
			Usage: "ocid container dir",
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

		containerDir := c.String("containerdir")
		sandboxDir := c.String("sandboxdir")
		service, err := server.New(c.String("runtime"), sandboxDir, containerDir)
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
