// securityprofile drives the security profile RPCs and the OCI security
// profile type (KEP-6061) for the integration tests, until crictl supports
// them. Configurations are read in the JSON format of crictl.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const usage = `Usage: securityprofile -r ENDPOINT COMMAND

Commands:
  features                          print whether the runtime reports seccomp_profile_oci
  pull [-kind KIND] [-namespace NS] REF
                                    pull a security profile and print whether it was cached
  list                              print the pulled security profiles as JSON
  remove DIGEST                     remove a pulled security profile
  runp SANDBOX_CONFIG               run a pod sandbox and print its ID
  create POD_ID CONTAINER_CONFIG SANDBOX_CONFIG
                                    create a container and print its ID
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	endpoint := flag.String("r", "unix:///var/run/crio/crio.sock", "CRI endpoint")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	flag.Parse()

	conn, err := grpc.NewClient(*endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to %s: %w", *endpoint, err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	runtime := types.NewRuntimeServiceClient(conn)
	args := flag.Args()

	switch {
	case len(args) == 1 && args[0] == "features":
		resp, err := runtime.Status(ctx, &types.StatusRequest{})
		if err != nil {
			return err
		}

		fmt.Printf("seccomp_profile_oci=%v\n", resp.GetFeatures().GetSeccompProfileOci())

	case len(args) >= 2 && args[0] == "pull":
		return pull(ctx, types.NewImageServiceClient(conn), args[1:])

	case len(args) == 1 && args[0] == "list":
		resp, err := types.NewImageServiceClient(conn).ListSecurityProfiles(
			ctx, &types.ListSecurityProfilesRequest{},
		)
		if err != nil {
			return err
		}

		data, err := protojson.Marshal(resp)
		if err != nil {
			return err
		}

		fmt.Println(string(data))

	case len(args) == 2 && args[0] == "remove":
		if _, err := types.NewImageServiceClient(conn).RemoveSecurityProfile(
			ctx, &types.RemoveSecurityProfileRequest{Digest: args[1]},
		); err != nil {
			return err
		}

	case len(args) == 2 && args[0] == "runp":
		config := &types.PodSandboxConfig{}
		if err := readConfig(args[1], config); err != nil {
			return err
		}

		resp, err := runtime.RunPodSandbox(ctx, &types.RunPodSandboxRequest{Config: config})
		if err != nil {
			return err
		}

		fmt.Println(resp.GetPodSandboxId())

	case len(args) == 4 && args[0] == "create":
		config := &types.ContainerConfig{}
		if err := readConfig(args[2], config); err != nil {
			return err
		}

		sandboxConfig := &types.PodSandboxConfig{}
		if err := readConfig(args[3], sandboxConfig); err != nil {
			return err
		}

		resp, err := runtime.CreateContainer(ctx, &types.CreateContainerRequest{
			PodSandboxId:  args[1],
			Config:        config,
			SandboxConfig: sandboxConfig,
		})
		if err != nil {
			return err
		}

		fmt.Println(resp.GetContainerId())

	default:
		flag.Usage()

		return errors.New("invalid arguments")
	}

	return nil
}

func pull(ctx context.Context, images types.ImageServiceClient, args []string) error {
	flags := flag.NewFlagSet("pull", flag.ExitOnError)
	kind := flags.String("kind", "Seccomp", "profile kind")
	namespace := flags.String("namespace", "", "namespace of the pod sandbox")

	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() != 1 {
		return fmt.Errorf("expected one reference, got %d", flags.NArg())
	}

	value, ok := types.SecurityProfileKind_value[*kind]
	if !ok {
		return fmt.Errorf("unknown profile kind %q", *kind)
	}

	ref := flags.Arg(0)

	resp, err := images.PullSecurityProfile(ctx, &types.PullSecurityProfileRequest{
		Image: &types.ImageSpec{Image: ref, UserSpecifiedImage: ref},
		SandboxConfig: &types.PodSandboxConfig{
			Metadata: &types.PodSandboxMetadata{Namespace: *namespace},
		},
		ProfileKind: types.SecurityProfileKind(value),
	})
	if err != nil {
		return err
	}

	fmt.Printf("cached=%v\n", resp.GetCached())

	return nil
}

func readConfig(path string, config proto.Message) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(
		data,
		config,
	); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	return nil
}
