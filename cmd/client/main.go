package main

import (
	"log"
	"os"

	pb "github.com/kubernetes/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	"github.com/urfave/cli"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

const (
	address = "localhost:49999"
)

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
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "runtimeversion",
			Value: "715fec664d75c6b5cb5b12718458621d4b75df37",
			Usage: "the version of the gPRC client API",
		},
	}
	app.Action = func(cxt *cli.Context) error {
		// Set up a connection to the server.
		conn, err := grpc.Dial(address, grpc.WithInsecure())
		if err != nil {
			log.Fatalf("Failed to connect: %v", err)
		}
		defer conn.Close()
		client := pb.NewRuntimeServiceClient(conn)

		// Test RuntimeServiceClient.Version
		err = Version(client, cxt.String("runtimeversion"))
		if err != nil {
			log.Fatalf("%s.Version failed: %v", app.Name, err)
		}

		return nil
	}

	app.Run(os.Args)
}
