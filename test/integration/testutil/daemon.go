package testutil

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

	"google.golang.org/grpc"

	"k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"

	"github.com/kubernetes-incubator/cri-o/server"
)

func StartServer() (*server.Config, error) {
	dir, err := ioutil.TempDir("", "ocid-")
	fmt.Printf("tempdir: %v\n", dir)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %v", err)
	}

	rootDir := filepath.Join(dir, "ocid")
	if err = os.Mkdir(rootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp root directory %q: %v", rootDir, err)
	}

	containerDir := filepath.Join(dir, "ocid", "containers")
	if err = os.Mkdir(containerDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp container directory %q: %v", containerDir, err)
	}

	sandboxDir := filepath.Join(dir, "sandboxes")
	if err = os.Mkdir(sandboxDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp sandbox directory %q: %v", sandboxDir, err)
	}

	config := server.DefaultConfig()
	config.RootConfig.Root = rootDir
	config.RootConfig.ContainerDir = containerDir
	config.RootConfig.SandboxDir = sandboxDir
	config.APIConfig.Listen = filepath.Join(dir, "ocid.sock")
	config.RuntimeConfig.Runtime = "/usr/bin/runc"
	config.RuntimeConfig.Conmon = "/home/jawnsy/cri-o/src/github.com/kubernetes-incubator/cri-o/conmon/conmon"
	config.ImageConfig.Pause = "/home/jawnsy/cri-o/src/github.com/kubernetes-incubator/cri-o/pause/pause"
	fmt.Printf("starting with config: %v\n", config)

	lis, err := net.Listen("unix", config.Listen)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err)
	}

	s := grpc.NewServer()

	service, err := server.New(config)
	if err != nil {
		return nil, err
	}

	runtime.RegisterRuntimeServiceServer(s, service)
	runtime.RegisterImageServiceServer(s, service)

	if err = s.Serve(lis); err != nil {
		return nil, fmt.Errorf("failed to Serve: %v", err)
	}

	return config, nil
}
