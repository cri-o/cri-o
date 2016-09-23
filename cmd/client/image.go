package main

import (
	"fmt"

	"github.com/urfave/cli"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

var imageCommand = cli.Command{
	Name: "image",
	Subcommands: []cli.Command{
		pullImageCommand,
	},
}

var pullImageCommand = cli.Command{
	Name:  "pull",
	Usage: "pull an image",
	Action: func(context *cli.Context) error {
		// Set up a connection to the server.
		conn, err := getClientConnection(context)
		if err != nil {
			return fmt.Errorf("failed to connect: %v", err)
		}
		defer conn.Close()
		client := pb.NewImageServiceClient(conn)

		_, err = PullImage(client, context.Args().Get(0))
		if err != nil {
			return fmt.Errorf("pulling image failed: %v", err)
		}
		return nil
	},
}

// PullImage sends a PullImageRequest to the server, and parses
// the returned ContainerStatusResponse.
func PullImage(client pb.ImageServiceClient, image string) (*pb.PullImageResponse, error) {
	return client.PullImage(context.Background(), &pb.PullImageRequest{Image: &pb.ImageSpec{Image: &image}})
}
