package server

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kubernetes-incubator/cri-o/libkpod"
	"github.com/kubernetes-incubator/cri-o/pkg/annotations"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
	"os"
)

func TestPrivilegedSandbox(t *testing.T) {
	testCases := map[string]struct {
		req      pb.RunPodSandboxRequest
		expected bool
	}{
		"Empty securityContext": {
			req: pb.RunPodSandboxRequest{
				Config: &pb.PodSandboxConfig{
					Linux: &pb.LinuxPodSandboxConfig{
						SecurityContext: &pb.LinuxSandboxSecurityContext{},
					},
				},
			},
			expected: false,
		},
		"securityContext.Privileged=true": {
			req: pb.RunPodSandboxRequest{
				Config: &pb.PodSandboxConfig{
					Linux: &pb.LinuxPodSandboxConfig{
						SecurityContext: &pb.LinuxSandboxSecurityContext{
							Privileged:       true,
							NamespaceOptions: &pb.NamespaceOption{},
						},
					},
				},
			},
			expected: true,
		},
		"securityContext.Privileged=false": {
			req: pb.RunPodSandboxRequest{
				Config: &pb.PodSandboxConfig{
					Linux: &pb.LinuxPodSandboxConfig{
						SecurityContext: &pb.LinuxSandboxSecurityContext{
							Privileged: false,
						},
					},
				},
			},
			expected: false,
		},
		"Empty namespaceOptions": {
			req: pb.RunPodSandboxRequest{
				Config: &pb.PodSandboxConfig{
					Linux: &pb.LinuxPodSandboxConfig{
						SecurityContext: &pb.LinuxSandboxSecurityContext{
							NamespaceOptions: &pb.NamespaceOption{},
						},
					},
				},
			},
			expected: false,
		},
		"namespaceOptions.HostNetwork=true": {
			req: pb.RunPodSandboxRequest{
				Config: &pb.PodSandboxConfig{
					Linux: &pb.LinuxPodSandboxConfig{
						SecurityContext: &pb.LinuxSandboxSecurityContext{
							Privileged: false,
							NamespaceOptions: &pb.NamespaceOption{
								HostNetwork: true,
								HostPid:     false,
								HostIpc:     false,
							},
						},
					},
				},
			},
			expected: true,
		},
		"namespaceOptions.HostPid=true": {
			req: pb.RunPodSandboxRequest{
				Config: &pb.PodSandboxConfig{
					Linux: &pb.LinuxPodSandboxConfig{
						SecurityContext: &pb.LinuxSandboxSecurityContext{
							Privileged: false,
							NamespaceOptions: &pb.NamespaceOption{
								HostNetwork: false,
								HostPid:     true,
								HostIpc:     false,
							},
						},
					},
				},
			},
			expected: true,
		},
		"namespaceOptions.HostIpc=true": {
			req: pb.RunPodSandboxRequest{
				Config: &pb.PodSandboxConfig{
					Linux: &pb.LinuxPodSandboxConfig{
						SecurityContext: &pb.LinuxSandboxSecurityContext{
							Privileged: false,
							NamespaceOptions: &pb.NamespaceOption{
								HostNetwork: false,
								HostPid:     false,
								HostIpc:     true,
							},
						},
					},
				},
			},
			expected: true,
		},
		"Both privileged & namespaceOptions is false": {
			req: pb.RunPodSandboxRequest{
				Config: &pb.PodSandboxConfig{
					Linux: &pb.LinuxPodSandboxConfig{
						SecurityContext: &pb.LinuxSandboxSecurityContext{
							Privileged: false,
							NamespaceOptions: &pb.NamespaceOption{
								HostNetwork: false,
								HostPid:     false,
								HostIpc:     false,
							},
						},
					},
				},
			},
			expected: false,
		},
	}
	s := &Server{}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result := s.privilegedSandbox(&tc.req)
			if result != tc.expected {
				t.Fatalf("%s expected %t but got %t", name, tc.expected, result)
			}
		})
	}
}

func TestTrustedSandbox(t *testing.T) {
	testCases := map[string]struct {
		req      pb.RunPodSandboxRequest
		expected bool
	}{
		"io.kubernetes.cri-o.TrustedSandbox=true": {
			req: pb.RunPodSandboxRequest{
				Config: &pb.PodSandboxConfig{
					Annotations: map[string]string{annotations.TrustedSandbox: "true"},
				},
			},
			expected: true,
		},
		"io.kubernetes.cri-o.TrustedSandbox=false": {
			req: pb.RunPodSandboxRequest{
				Config: &pb.PodSandboxConfig{
					Annotations: map[string]string{annotations.TrustedSandbox: "false"},
				},
			},
			expected: false,
		},
		"A sandbox is trusted by default": {
			req: pb.RunPodSandboxRequest{
				Config: &pb.PodSandboxConfig{
					Annotations: map[string]string{"test": "test"},
				},
			},
			expected: true,
		},
		"Annotations is null": {
			req: pb.RunPodSandboxRequest{
				Config: &pb.PodSandboxConfig{},
			},
			expected: true,
		},
	}
	s := &Server{}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result := s.trustedSandbox(&tc.req)
			if result != tc.expected {
				t.Fatalf("%s expected %t but got %t", name, tc.expected, result)
			}
		})
	}
}

type fakeServer struct {
	*Server
}

func newFakeServer() (*fakeServer, error) {
	c := libkpod.DefaultConfig()
	apiConfig := APIConfig{}
	s, err := New(&Config{*c, apiConfig})
	if err != nil {
		return nil, err
	}
	return &fakeServer{s}, nil
}

func (s *fakeServer) cleanPodSandbox(ctx context.Context, podSandboxID string) error {
	if _, err := s.StopPodSandbox(ctx, &pb.StopPodSandboxRequest{PodSandboxId: podSandboxID}); err != nil {
		fmt.Println("fakeServer StopPodSandbox get error: %v", err)
		return err
	}
	if _, err := s.RemovePodSandbox(ctx, &pb.RemovePodSandboxRequest{PodSandboxId: podSandboxID}); err != nil {
		fmt.Println("fakeServer RemovePodSandbox get error: %v", err)
		return err
	}
	return nil
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

func TestRunPodSandbox(t *testing.T) {
	s, err := newFakeServer()
	if err != nil {
		t.Fatalf("Create fakeServer error, %v", err)
	}
	ctx := context.Background()

	//when name is empty, should get "PodSandboxConfig.Name should not be empty" error
	sdconf, _ := loadPodSandboxConfig("fixtures/sandbox_config.json")
	req := &pb.RunPodSandboxRequest{Config: sdconf}
	req.Config.Metadata.Name = ""
	fmt.Println("RunPodSandboxRequest: 0")
	resp, err := s.RunPodSandbox(ctx, req)
	if resp != nil && err.Error() != "PodSandboxConfig.Name should not be empty" {
		t.Fatalf("RunPodSandbox should fail when name is empty")
	}

	//run with same sandbox_configure twice should also create sandbox success
	sdconf1, _ := loadPodSandboxConfig("fixtures/sandbox_config.json")
	req1 := &pb.RunPodSandboxRequest{Config: sdconf1}
	resp, err = s.RunPodSandbox(ctx, req1)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = s.RunPodSandbox(ctx, req1)
	if err != nil {
		t.Fatalf("Run with same sandbox_configure twice %v", err)
	}
	s.cleanPodSandbox(ctx, resp.PodSandboxId)
}
