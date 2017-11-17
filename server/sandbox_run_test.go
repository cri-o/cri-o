package server

import (
	"github.com/kubernetes-incubator/cri-o/pkg/annotations"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
	"testing"
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
