package main

import (
	"encoding/json"
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

func openFile(path string) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config at %s not found", path)
		}
		return nil, err
	}
	return f, nil
}

func loadPodSandboxConfig(path string) (*pb.PodSandboxConfig, error) {
	f, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var config pb.PodSandboxConfig
	if err := json.NewDecoder(f).Decode(&config); err != nil {
		return nil, err
	}
	return &config, nil
}

func loadContainerConfig(path string) (*pb.ContainerConfig, error) {
	f, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var config pb.ContainerConfig
	if err := json.NewDecoder(f).Decode(&config); err != nil {
		return nil, err
	}
	return &config, nil
}

// CreatePodSandbox sends a CreatePodSandboxRequest to the server, and parses
// the returned CreatePodSandboxResponse.
func CreatePodSandbox(client pb.RuntimeServiceClient, path string) error {
	config, err := loadPodSandboxConfig(path)
	if err != nil {
		return err
	}

	r, err := client.CreatePodSandbox(context.Background(), &pb.CreatePodSandboxRequest{Config: config})
	if err != nil {
		return err
	}
	fmt.Println(r)
	return nil
}

// StopPodSandbox sends a StopPodSandboxRequest to the server, and parses
// the returned StopPodSandboxResponse.
func StopPodSandbox(client pb.RuntimeServiceClient, ID string) error {
	if ID == "" {
		return fmt.Errorf("ID cannot be empty")
	}
	r, err := client.StopPodSandbox(context.Background(), &pb.StopPodSandboxRequest{PodSandboxId: &ID})
	if err != nil {
		return err
	}
	fmt.Println(r)
	return nil
}

// RemovePodSandbox sends a RemovePodSandboxRequest to the server, and parses
// the returned RemovePodSandboxResponse.
func RemovePodSandbox(client pb.RuntimeServiceClient, ID string) error {
	if ID == "" {
		return fmt.Errorf("ID cannot be empty")
	}
	r, err := client.RemovePodSandbox(context.Background(), &pb.RemovePodSandboxRequest{PodSandboxId: &ID})
	if err != nil {
		return err
	}
	fmt.Println(r)
	return nil
}

// CreateContainer sends a CreateContainerRequest to the server, and parses
// the returned CreateContainerResponse.
func CreateContainer(client pb.RuntimeServiceClient, sandbox string, path string) error {
	config, err := loadContainerConfig(path)
	if err != nil {
		return err
	}

	r, err := client.CreateContainer(context.Background(), &pb.CreateContainerRequest{
		PodSandboxId: &sandbox,
		Config:       config,
	})
	if err != nil {
		return err
	}
	fmt.Println(r)
	return nil
}

// StartContainer sends a StartContainerRequest to the server, and parses
// the returned StartContainerResponse.
func StartContainer(client pb.RuntimeServiceClient, ID string) error {
	if ID == "" {
		return fmt.Errorf("ID cannot be empty")
	}
	r, err := client.StartContainer(context.Background(), &pb.StartContainerRequest{
		ContainerId: &ID,
	})
	if err != nil {
		return err
	}
	fmt.Println(r)
	return nil
}

// StopContainer sends a StopContainerRequest to the server, and parses
// the returned StopContainerResponse.
func StopContainer(client pb.RuntimeServiceClient, ID string) error {
	if ID == "" {
		return fmt.Errorf("ID cannot be empty")
	}
	r, err := client.StopContainer(context.Background(), &pb.StopContainerRequest{
		ContainerId: &ID,
	})
	if err != nil {
		return err
	}
	fmt.Println(r)
	return nil
}

// RemoveContainer sends a RemoveContainerRequest to the server, and parses
// the returned RemoveContainerResponse.
func RemoveContainer(client pb.RuntimeServiceClient, ID string) error {
	if ID == "" {
		return fmt.Errorf("ID cannot be empty")
	}
	r, err := client.RemoveContainer(context.Background(), &pb.RemoveContainerRequest{
		ContainerId: &ID,
	})
	if err != nil {
		return err
	}
	fmt.Println(r)
	return nil
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
	app.Version = "0.0.1"

	app.Commands = []cli.Command{
		podSandboxCommand,
		containerCommand,
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

var podSandboxCommand = cli.Command{
	Name: "pod",
	Subcommands: []cli.Command{
		createPodSandboxCommand,
		stopPodSandboxCommand,
		removePodSandboxCommand,
	},
}

var createPodSandboxCommand = cli.Command{
	Name:  "create",
	Usage: "create a pod",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "config",
			Value: "config.json",
			Usage: "the path of a pod sandbox config file",
		},
	},
	Action: func(context *cli.Context) error {
		// Set up a connection to the server.
		conn, err := getClientConnection()
		if err != nil {
			return fmt.Errorf("Failed to connect: %v", err)
		}
		defer conn.Close()
		client := pb.NewRuntimeServiceClient(conn)

		// Test RuntimeServiceClient.CreatePodSandbox
		err = CreatePodSandbox(client, context.String("config"))
		if err != nil {
			return fmt.Errorf("Creating the pod sandbox failed: %v", err)
		}
		return nil
	},
}

var stopPodSandboxCommand = cli.Command{
	Name:  "stop",
	Usage: "stop a pod sandbox",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "id",
			Value: "",
			Usage: "id of the pod sandbox",
		},
	},
	Action: func(context *cli.Context) error {
		// Set up a connection to the server.
		conn, err := getClientConnection()
		if err != nil {
			return fmt.Errorf("Failed to connect: %v", err)
		}
		defer conn.Close()
		client := pb.NewRuntimeServiceClient(conn)

		err = StopPodSandbox(client, context.String("id"))
		if err != nil {
			return fmt.Errorf("Stopping the pod sandbox failed: %v", err)
		}
		return nil
	},
}

var removePodSandboxCommand = cli.Command{
	Name:  "remove",
	Usage: "remove a pod sandbox",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "id",
			Value: "",
			Usage: "id of the pod sandbox",
		},
	},
	Action: func(context *cli.Context) error {
		// Set up a connection to the server.
		conn, err := getClientConnection()
		if err != nil {
			return fmt.Errorf("failed to connect: %v", err)
		}
		defer conn.Close()
		client := pb.NewRuntimeServiceClient(conn)

		err = RemovePodSandbox(client, context.String("id"))
		if err != nil {
			return fmt.Errorf("removing the pod sandbox failed: %v", err)
		}
		return nil
	},
}

var containerCommand = cli.Command{
	Name:    "container",
	Aliases: []string{"ctr"},
	Subcommands: []cli.Command{
		createContainerCommand,
		startContainerCommand,
		stopContainerCommand,
		removeContainerCommand,
	},
}

var createContainerCommand = cli.Command{
	Name:  "create",
	Usage: "create a container",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "pod",
			Usage: "the id of the pod sandbox to which the container belongs",
		},
		cli.StringFlag{
			Name:  "config",
			Value: "config.json",
			Usage: "the path of a container config file",
		},
	},
	Action: func(context *cli.Context) error {
		// Set up a connection to the server.
		conn, err := getClientConnection()
		if err != nil {
			return fmt.Errorf("Failed to connect: %v", err)
		}
		defer conn.Close()
		client := pb.NewRuntimeServiceClient(conn)

		if !context.IsSet("pod") {
			return fmt.Errorf("Please specify the id of the pod sandbox to which the container belongs via the --pod option")
		}
		// Test RuntimeServiceClient.CreateContainer
		err = CreateContainer(client, context.String("pod"), context.String("config"))
		if err != nil {
			return fmt.Errorf("Creating the pod sandbox failed: %v", err)
		}
		return nil
	},
}

var startContainerCommand = cli.Command{
	Name:  "start",
	Usage: "start a container",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "id",
			Value: "",
			Usage: "id of the container",
		},
	},
	Action: func(context *cli.Context) error {
		// Set up a connection to the server.
		conn, err := getClientConnection()
		if err != nil {
			return fmt.Errorf("Failed to connect: %v", err)
		}
		defer conn.Close()
		client := pb.NewRuntimeServiceClient(conn)

		err = StartContainer(client, context.String("id"))
		if err != nil {
			return fmt.Errorf("Starting the container failed: %v", err)
		}
		return nil
	},
}

var stopContainerCommand = cli.Command{
	Name:  "stop",
	Usage: "stop a container",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "id",
			Value: "",
			Usage: "id of the container",
		},
	},
	Action: func(context *cli.Context) error {
		// Set up a connection to the server.
		conn, err := getClientConnection()
		if err != nil {
			return fmt.Errorf("Failed to connect: %v", err)
		}
		defer conn.Close()
		client := pb.NewRuntimeServiceClient(conn)

		err = StopContainer(client, context.String("id"))
		if err != nil {
			return fmt.Errorf("Stopping the container failed: %v", err)
		}
		return nil
	},
}

var removeContainerCommand = cli.Command{
	Name:  "remove",
	Usage: "remove a container",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "id",
			Value: "",
			Usage: "id of the container",
		},
	},
	Action: func(context *cli.Context) error {
		// Set up a connection to the server.
		conn, err := getClientConnection()
		if err != nil {
			return fmt.Errorf("Failed to connect: %v", err)
		}
		defer conn.Close()
		client := pb.NewRuntimeServiceClient(conn)

		err = RemoveContainer(client, context.String("id"))
		if err != nil {
			return fmt.Errorf("Removing the container failed: %v", err)
		}
		return nil
	},
}
