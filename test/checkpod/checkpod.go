package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	cri "k8s.io/cri-client/pkg"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: checkpod supported|checkpoint|restore|cancel [options]")
	}

	switch args[0] {
	case "supported":
		return supported(args[1:])
	case "checkpoint":
		return checkpoint(args[1:])
	case "restore":
		return restore(args[1:])
	case "cancel":
		return cancelOperation(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func checkpoint(args []string) error {
	flags := flag.NewFlagSet("checkpoint", flag.ContinueOnError)
	endpoint := flags.String("endpoint", "", "CRI runtime endpoint")
	sandboxID := flags.String("sandbox", "", "pod sandbox ID")
	output := flags.String("output", "", "checkpoint output directory")
	containerIDs := flags.String("containers", "", "comma-separated container IDs")
	operationID := flags.String("operation-id", "", "operation ID, generated when empty")

	timeout := flags.Duration("timeout", 30*time.Second, "checkpoint timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if *endpoint == "" || *sandboxID == "" || *output == "" || *containerIDs == "" {
		return errors.New("endpoint, sandbox, output, and containers are required")
	}

	service, err := cri.NewRemoteRuntimeServiceBuilder().
		WithEndpoint(*endpoint).
		WithConnectionTimeout(*timeout).
		WithTracerProvider(noop.NewTracerProvider()).
		Build(context.Background())
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	return service.CheckpointPod(ctx, &runtimeapi.CheckpointPodRequest{
		OperationId:  operationIdentity(*operationID),
		PodSandboxId: *sandboxID,
		OutputPath:   *output,
		ContainerIds: strings.Split(*containerIDs, ","),
	})
}

func restore(args []string) error {
	flags := flag.NewFlagSet("restore", flag.ContinueOnError)
	endpoint := flags.String("endpoint", "", "CRI runtime endpoint")
	checkpointPath := flags.String("checkpoint", "", "checkpoint directory")
	sandboxConfigPath := flags.String("sandbox-config", "", "pod sandbox config JSON")
	containerConfigPaths := flags.String(
		"container-configs",
		"",
		"comma-separated container config JSON files",
	)
	runtimeHandler := flags.String("runtime-handler", "", "runtime handler")
	operationID := flags.String("operation-id", "", "operation ID, generated when empty")

	timeout := flags.Duration("timeout", 2*time.Minute, "restore timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if *endpoint == "" || *checkpointPath == "" || *sandboxConfigPath == "" ||
		*containerConfigPaths == "" {
		return errors.New(
			"endpoint, checkpoint, sandbox-config, and container-configs are required",
		)
	}

	sandboxConfig := new(runtimeapi.PodSandboxConfig)
	if err := readJSON(*sandboxConfigPath, sandboxConfig); err != nil {
		return err
	}

	containerConfigs := make([]*runtimeapi.ContainerConfig, 0)

	for path := range strings.SplitSeq(*containerConfigPaths, ",") {
		config := new(runtimeapi.ContainerConfig)
		if err := readJSON(path, config); err != nil {
			return err
		}

		containerConfigs = append(containerConfigs, config)
	}

	service, err := cri.NewRemoteRuntimeServiceBuilder().
		WithEndpoint(*endpoint).
		WithConnectionTimeout(*timeout).
		WithTracerProvider(noop.NewTracerProvider()).
		Build(context.Background())
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	response, err := service.RestorePod(ctx, &runtimeapi.RestorePodRequest{
		OperationId:      operationIdentity(*operationID),
		CheckpointPath:   *checkpointPath,
		Config:           sandboxConfig,
		RuntimeHandler:   *runtimeHandler,
		ContainerConfigs: containerConfigs,
	})
	if err != nil {
		return err
	}

	return json.NewEncoder(os.Stdout).Encode(response)
}

// supported exits successfully only when the runtime advertises the Pod
// checkpoint and restore RPCs, so tests can skip runtimes without them.
func supported(args []string) error {
	flags := flag.NewFlagSet("supported", flag.ContinueOnError)
	endpoint := flags.String("endpoint", "", "CRI runtime endpoint")

	timeout := flags.Duration("timeout", 30*time.Second, "status timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if *endpoint == "" {
		return errors.New("endpoint is required")
	}

	service, err := cri.NewRemoteRuntimeServiceBuilder().
		WithEndpoint(*endpoint).
		WithConnectionTimeout(*timeout).
		WithTracerProvider(noop.NewTracerProvider()).
		Build(context.Background())
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	status, err := service.Status(ctx, false)
	if err != nil {
		return err
	}

	if !status.GetFeatures().GetPodCheckpointRestoreCancellation() {
		return errors.New("runtime does not support Pod checkpoint and restore")
	}

	return nil
}

// cancelOperation settles an interrupted operation. The CRI client library
// does not wrap CancelPodOperation, so this uses the generated gRPC client.
func cancelOperation(args []string) error {
	flags := flag.NewFlagSet("cancel", flag.ContinueOnError)
	endpoint := flags.String("endpoint", "", "CRI runtime endpoint")
	operationID := flags.String("operation-id", "", "operation ID")
	operationType := flags.String("type", "", "checkpoint or restore")

	timeout := flags.Duration("timeout", 2*time.Minute, "cancellation timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}

	kinds := map[string]runtimeapi.PodOperationType{
		"checkpoint": runtimeapi.PodOperationType_POD_CHECKPOINT,
		"restore":    runtimeapi.PodOperationType_POD_RESTORE,
	}

	kind, ok := kinds[*operationType]
	if *endpoint == "" || *operationID == "" || !ok {
		return errors.New("endpoint, operation-id, and type checkpoint or restore are required")
	}

	conn, err := grpc.NewClient(*endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	_, err = runtimeapi.NewRuntimeServiceClient(conn).CancelPodOperation(
		ctx,
		&runtimeapi.CancelPodOperationRequest{OperationId: *operationID, OperationType: kind},
	)

	return err
}

func operationIdentity(id string) string {
	if id != "" {
		return id
	}

	return rand.Text()
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}

	return nil
}
