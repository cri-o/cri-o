package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: %s <socket> <container-id> <key=value>", os.Args[0])
	}

	socket := os.Args[1]
	containerID := os.Args[2]
	unified := make(map[string]string)

	for _, arg := range os.Args[3:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid key=value pair: %s", arg)
		}

		unified[parts[0]] = parts[1]
	}

	conn, err := grpc.NewClient(
		"unix://"+socket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to create gRPC client: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := runtimeapi.NewRuntimeServiceClient(conn)

	_, err = client.UpdateContainerResources(ctx, &runtimeapi.UpdateContainerResourcesRequest{
		ContainerId: containerID,
		Linux: &runtimeapi.LinuxContainerResources{
			Unified: unified,
		},
	})
	if err != nil {
		return fmt.Errorf("UpdateContainerResources failed: %w", err)
	}

	return nil
}
