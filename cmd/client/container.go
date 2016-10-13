package main

import (
	"fmt"
	"log"
	"time"

	"github.com/urfave/cli"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

var containerCommand = cli.Command{
	Name:    "container",
	Aliases: []string{"ctr"},
	Subcommands: []cli.Command{
		createContainerCommand,
		startContainerCommand,
		stopContainerCommand,
		removeContainerCommand,
		containerStatusCommand,
		listContainersCommand,
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
		cli.StringFlag{
			Name:  "name",
			Value: "",
			Usage: "the name of the container",
		},
	},
	Action: func(context *cli.Context) error {
		// Set up a connection to the server.
		conn, err := getClientConnection(context)
		if err != nil {
			return fmt.Errorf("failed to connect: %v", err)
		}
		defer conn.Close()
		client := pb.NewRuntimeServiceClient(conn)

		if !context.IsSet("pod") {
			return fmt.Errorf("Please specify the id of the pod sandbox to which the container belongs via the --pod option")
		}
		// Test RuntimeServiceClient.CreateContainer
		err = CreateContainer(client, context.String("pod"), context.String("config"), context.String("name"))
		if err != nil {
			return fmt.Errorf("Creating container failed: %v", err)
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
		conn, err := getClientConnection(context)
		if err != nil {
			return fmt.Errorf("failed to connect: %v", err)
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
		conn, err := getClientConnection(context)
		if err != nil {
			return fmt.Errorf("failed to connect: %v", err)
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
		conn, err := getClientConnection(context)
		if err != nil {
			return fmt.Errorf("failed to connect: %v", err)
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

var containerStatusCommand = cli.Command{
	Name:  "status",
	Usage: "get the status of a container",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "id",
			Value: "",
			Usage: "id of the container",
		},
	},
	Action: func(context *cli.Context) error {
		// Set up a connection to the server.
		conn, err := getClientConnection(context)
		if err != nil {
			return fmt.Errorf("failed to connect: %v", err)
		}
		defer conn.Close()
		client := pb.NewRuntimeServiceClient(conn)

		err = ContainerStatus(client, context.String("id"))
		if err != nil {
			return fmt.Errorf("Getting the status of the container failed: %v", err)
		}
		return nil
	},
}

var listContainersCommand = cli.Command{
	Name:  "list",
	Usage: "list containers",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "quiet",
			Usage: "list only container IDs",
		},
		cli.StringFlag{
			Name:  "id",
			Value: "",
			Usage: "filter by container id",
		},
		cli.StringFlag{
			Name:  "pod",
			Value: "",
			Usage: "filter by container pod id",
		},
		cli.StringFlag{
			Name:  "state",
			Value: "",
			Usage: "filter by container state",
		},
	},
	Action: func(context *cli.Context) error {
		// Set up a connection to the server.
		conn, err := getClientConnection(context)
		if err != nil {
			return fmt.Errorf("failed to connect: %v", err)
		}
		defer conn.Close()
		client := pb.NewRuntimeServiceClient(conn)

		err = ListContainers(client, context.Bool("quiet"), context.String("id"), context.String("pod"), context.String("state"))
		if err != nil {
			return fmt.Errorf("listing containers failed: %v", err)
		}
		return nil
	},
}

// CreateContainer sends a CreateContainerRequest to the server, and parses
// the returned CreateContainerResponse.
func CreateContainer(client pb.RuntimeServiceClient, sandbox string, path string, name string) error {
	config, err := loadContainerConfig(path)
	if err != nil {
		return err
	}

	// Override the name by the one specified through CLI
	if name != "" {
		config.Metadata.Name = &name
	}

	r, err := client.CreateContainer(context.Background(), &pb.CreateContainerRequest{
		PodSandboxId: &sandbox,
		Config:       config,
	})
	if err != nil {
		return err
	}
	fmt.Println(*r.ContainerId)
	return nil
}

// StartContainer sends a StartContainerRequest to the server, and parses
// the returned StartContainerResponse.
func StartContainer(client pb.RuntimeServiceClient, ID string) error {
	if ID == "" {
		return fmt.Errorf("ID cannot be empty")
	}
	_, err := client.StartContainer(context.Background(), &pb.StartContainerRequest{
		ContainerId: &ID,
	})
	if err != nil {
		return err
	}
	fmt.Println(ID)
	return nil
}

// StopContainer sends a StopContainerRequest to the server, and parses
// the returned StopContainerResponse.
func StopContainer(client pb.RuntimeServiceClient, ID string) error {
	if ID == "" {
		return fmt.Errorf("ID cannot be empty")
	}
	_, err := client.StopContainer(context.Background(), &pb.StopContainerRequest{
		ContainerId: &ID,
	})
	if err != nil {
		return err
	}
	fmt.Println(ID)
	return nil
}

// RemoveContainer sends a RemoveContainerRequest to the server, and parses
// the returned RemoveContainerResponse.
func RemoveContainer(client pb.RuntimeServiceClient, ID string) error {
	if ID == "" {
		return fmt.Errorf("ID cannot be empty")
	}
	_, err := client.RemoveContainer(context.Background(), &pb.RemoveContainerRequest{
		ContainerId: &ID,
	})
	if err != nil {
		return err
	}
	fmt.Println(ID)
	return nil
}

// ContainerStatus sends a ContainerStatusRequest to the server, and parses
// the returned ContainerStatusResponse.
func ContainerStatus(client pb.RuntimeServiceClient, ID string) error {
	if ID == "" {
		return fmt.Errorf("ID cannot be empty")
	}
	r, err := client.ContainerStatus(context.Background(), &pb.ContainerStatusRequest{
		ContainerId: &ID})
	if err != nil {
		return err
	}
	fmt.Printf("ID: %s\n", *r.Status.Id)
	if r.Status.State != nil {
		fmt.Printf("Status: %s\n", r.Status.State)
	}
	if r.Status.CreatedAt != nil {
		ctm := time.Unix(*r.Status.CreatedAt, 0)
		fmt.Printf("Created: %v\n", ctm)
	}
	if r.Status.StartedAt != nil {
		stm := time.Unix(*r.Status.StartedAt, 0)
		fmt.Printf("Started: %v\n", stm)
	}
	if r.Status.FinishedAt != nil {
		ftm := time.Unix(*r.Status.FinishedAt, 0)
		fmt.Printf("Finished: %v\n", ftm)
	}
	if r.Status.ExitCode != nil {
		fmt.Printf("Exit Code: %v\n", *r.Status.ExitCode)
	}

	return nil
}

// ListContainers sends a ListContainerRequest to the server, and parses
// the returned ListContainerResponse.
func ListContainers(client pb.RuntimeServiceClient, quiet bool, id string, podID string, state string) error {
	filter := &pb.ContainerFilter{}
	if id != "" {
		filter.Id = &id
	}
	if podID != "" {
		filter.PodSandboxId = &podID
	}
	if state != "" {
		st := pb.ContainerState_UNKNOWN
		switch state {
		case "created":
			st = pb.ContainerState_CREATED
			filter.State = &st
		case "running":
			st = pb.ContainerState_RUNNING
			filter.State = &st
		case "stopped":
			st = pb.ContainerState_EXITED
			filter.State = &st
		default:
			log.Fatalf("--state should be one of created, running or stopped")
		}
	}
	r, err := client.ListContainers(context.Background(), &pb.ListContainersRequest{
		Filter: filter,
	})
	if err != nil {
		return err
	}
	for _, c := range r.GetContainers() {
		if quiet {
			fmt.Println(*c.Id)
			continue
		}
		fmt.Printf("ID: %s\n", *c.Id)
		fmt.Printf("Pod: %s\n", *c.PodSandboxId)
		if c.State != nil {
			fmt.Printf("Status: %s\n", *c.State)
		}
		if c.CreatedAt != nil {
			ctm := time.Unix(*c.CreatedAt, 0)
			fmt.Printf("Created: %v\n", ctm)
		}
	}
	return nil
}
