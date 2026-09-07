package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace/noop"
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
		return errors.New("usage: checkpod checkpoint|restore [options]")
	}

	switch args[0] {
	case "checkpoint":
		return checkpoint(args[1:])
	case "restore":
		return restore(args[1:])
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

	timeout := flags.Duration("timeout", 30*time.Second, "checkpoint timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if *endpoint == "" || *sandboxID == "" || *output == "" || *containerIDs == "" {
		return errors.New("endpoint, sandbox, output, and containers are required")
	}

	service, err := cri.NewRemoteRuntimeService(context.Background(), *endpoint, *timeout, noop.NewTracerProvider(), false)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	return service.CheckpointPod(ctx, &runtimeapi.CheckpointPodRequest{
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
	containerConfigPaths := flags.String("container-configs", "", "comma-separated container config JSON files")
	runtimeHandler := flags.String("runtime-handler", "", "runtime handler")

	timeout := flags.Duration("timeout", 2*time.Minute, "restore timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if *endpoint == "" || *checkpointPath == "" || *sandboxConfigPath == "" || *containerConfigPaths == "" {
		return errors.New("endpoint, checkpoint, sandbox-config, and container-configs are required")
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

	service, err := cri.NewRemoteRuntimeService(context.Background(), *endpoint, *timeout, noop.NewTracerProvider(), false)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	response, err := service.RestorePod(ctx, &runtimeapi.RestorePodRequest{
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
