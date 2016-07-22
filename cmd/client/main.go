package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"

	pb "github.com/kubernetes/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	"github.com/urfave/cli"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

const (
	unixDomainSocket = "/var/run/ocid.sock"
	// TODO: Make configurable
	timeout = 10 * time.Second
)

func getClientConnection() (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(unixDomainSocket, grpc.WithInsecure(), grpc.WithTimeout(timeout),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}))
	if err != nil {
		return nil, fmt.Errorf("Failed to connect: %v", err)
	}
	return conn, nil
}

// Version sends a VersionRequest to the server, and parses the returned VersionResponse.
func Version(client pb.RuntimeServiceClient, version string) error {
	r, err := client.Version(context.Background(), &pb.VersionRequest{Version: &version})
	if err != nil {
		return err
	}
	log.Printf("VersionResponse: Version: %s, RuntimeName: %s, RuntimeVersion: %s, RuntimeApiVersion: %s\n", *r.Version, *r.RuntimeName, *r.RuntimeVersion, *r.RuntimeApiVersion)
	return nil
}

func main() {
	app := cli.NewApp()
	app.Name = "ocic"
	app.Usage = "client for ocid"

	app.Commands = []cli.Command{
		runtimeVersionCommand,
		pullImageCommand,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func PullImage(client pb.ImageServiceClient, image string) error {
	_, err := client.PullImage(context.Background(), &pb.PullImageRequest{Image: &pb.ImageSpec{Image: &image}})
	if err != nil {
		return err
	}
	return nil
}

// try this with ./ocic pullimage docker://busybox
var pullImageCommand = cli.Command{
	Name:  "pullimage",
	Usage: "pull an image",
	Action: func(context *cli.Context) error {
		// Set up a connection to the server.
		conn, err := getClientConnection()
		if err != nil {
			return fmt.Errorf("Failed to connect: %v", err)
		}
		defer conn.Close()
		client := pb.NewImageServiceClient(conn)

		err = PullImage(client, context.Args().Get(0))
		if err != nil {
			return fmt.Errorf("pulling image failed: %v", err)
		}
		return nil
	},
}

var runtimeVersionCommand = cli.Command{
	Name:  "runtimeversion",
	Usage: "get runtime version information",
	Action: func(context *cli.Context) error {
		// Set up a connection to the server.
		conn, err := getClientConnection()
		if err != nil {
			return fmt.Errorf("Failed to connect: %v", err)
		}
		defer conn.Close()
		client := pb.NewRuntimeServiceClient(conn)

		// Test RuntimeServiceClient.Version
		version := "v1alpha1"
		err = Version(client, version)
		if err != nil {
			return fmt.Errorf("Getting the runtime version failed: %v", err)
		}
		return nil
	},
}
