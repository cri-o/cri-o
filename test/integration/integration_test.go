package integration_test

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"

	"golang.org/x/net/context"

	"github.com/kubernetes-incubator/cri-o/test/integration/testutil"
	gk "github.com/onsi/ginkgo"
	gm "github.com/onsi/gomega"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

func TestBooks(t *testing.T) {
	gm.RegisterFailHandler(gk.Fail)

	t.Log("Running tests!")
	gk.RunSpecs(t, "CRI-O Integration Test Suite")
}

var _ = gk.Describe("CRI-O Integration Test Suite", func() {
	gk.It("should start the server", func() {
		fmt.Println("starting server")
		_, err := testutil.StartServer()
		if err != nil {
			fmt.Printf("error starting: %v\n", err)
		}

		time.Sleep(10 * time.Minute)

		/*
			conn, err := getClientConnection(config.APIConfig.Listen)
			if err != nil {
				fmt.Printf("error connecting: %v\n")
			}

			client := pb.NewRuntimeServiceClient(conn)

			opts := createOptions{
				configPath: context.String("config"),
				name:       context.String("name"),
				labels:     map[string]string{},
			}

			for _, l := range context.StringSlice("label") {
				pair := strings.Split(l, "=")
				if len(pair) != 2 {
					return fmt.Errorf("incorrectly specified label: %v", l)
				}
				opts.labels[pair[0]] = pair[1]
			}

			// Test RuntimeServiceClient.RunPodSandbox
			err = RunPodSandbox(client, opts)
			if err != nil {
				return fmt.Errorf("Creating the pod sandbox failed: %v", err)
			}
		*/
	})

	/*
		start_ocid
		run ocic pod run --config "$TESTDATA"/sandbox_config.json
		echo "$output"
		[ "$status" -eq 0 ]
		pod_id="$output"
		run ocic ctr create --config "$TESTDATA"/container_redis.json --pod "$pod_id"
		echo "$output"
		[ "$status" -eq 0 ]
		ctr_id="$output"
		run ocic ctr start --id "$ctr_id"
		echo "$output"
		[ "$status" -eq 0 ]
		run ocic ctr remove --id "$ctr_id"
		echo "$output"
		[ "$status" -eq 0 ]
		run ocic pod stop --id "$pod_id"
		echo "$output"
		[ "$status" -eq 0 ]
		run ocic pod remove --id "$pod_id"
		echo "$output"
		[ "$status" -eq 0 ]
		cleanup_ctrs
		cleanup_pods
		stop_ocid
	*/

})

type createOptions struct {
	// configPath is path to the config for container
	configPath string
	// name sets the container name
	name string
	// podID of the container
	podID string
	// labels for the container
	labels map[string]string
}

// RunPodSandbox sends a RunPodSandboxRequest to the server, and parses
// the returned RunPodSandboxResponse.
func RunPodSandbox(client pb.RuntimeServiceClient, opts createOptions) error {
	config, err := loadPodSandboxConfig(opts.configPath)
	if err != nil {
		return err
	}

	// Override the name by the one specified through CLI
	if opts.name != "" {
		config.Metadata.Name = &opts.name
	}

	for k, v := range opts.labels {
		config.Labels[k] = v
	}

	r, err := client.RunPodSandbox(context.Background(), &pb.RunPodSandboxRequest{Config: config})
	if err != nil {
		return err
	}
	fmt.Println(*r.PodSandboxId)
	return nil
}

func getClientConnection(addr string) (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithTimeout(30*time.Second),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}))
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %v", err)
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
