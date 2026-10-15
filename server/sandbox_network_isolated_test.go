package server

import (
	"context"
	"strings"
	"testing"
	"time"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/memorystore"
	"github.com/cri-o/cri-o/internal/oci"
)

// TestIsNetworkIsolated mirrors containerd's TestisNetworkIsolated for the
// KEP-6313 Non Network Pods port: DEFAULT_NETWORK_NONE opts out of the
// default pod network plumbing, everything else keeps historic behavior.
func TestIsNetworkIsolated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   *types.PodSandboxConfig
		expected bool
	}{
		{
			name:     "when config is nil return false",
			config:   nil,
			expected: false,
		},
		{
			name:     "when default network is unset return false",
			config:   &types.PodSandboxConfig{},
			expected: false,
		},
		{
			name: "when default network is pod return false",
			config: &types.PodSandboxConfig{
				DefaultNetwork: types.PodSandboxDefaultNetwork_DEFAULT_NETWORK_POD,
			},
			expected: false,
		},
		{
			name: "when default network is none return true",
			config: &types.PodSandboxConfig{
				DefaultNetwork: types.PodSandboxDefaultNetwork_DEFAULT_NETWORK_NONE,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isNetworkIsolated(tt.config); got != tt.expected {
				t.Errorf("isNetworkIsolated() = %t, expected %t", got, tt.expected)
			}
		})
	}
}

// newNetworkIsolatedTestSandbox builds a minimal sandbox with the
// network-isolated flag set, mirroring the server suite's beforeEach fixture.
func newNetworkIsolatedTestSandbox(t *testing.T, networkIsolated bool) *sandbox.Sandbox {
	t.Helper()

	sbox := sandbox.NewBuilder()
	sbox.SetID("sandboxID")
	sbox.SetName("sandboxName")
	sbox.SetLogDir("test")
	sbox.SetShmPath("test")
	sbox.SetNamespace("")
	sbox.SetKubeName("")
	sbox.SetMountLabel("")
	sbox.SetProcessLabel("")
	sbox.SetCgroupParent("")
	sbox.SetRuntimeHandler("")
	sbox.SetResolvPath("")
	sbox.SetHostname("")
	sbox.SetPortMappings([]*hostport.PortMapping{})
	sbox.SetHostNetwork(false)
	sbox.SetNetworkIsolated(networkIsolated)
	sbox.SetUsernsMode("")
	sbox.SetPodLinuxOverhead(nil)
	sbox.SetPodLinuxResources(nil)
	sbox.SetCreatedAt(time.Now())

	if err := sbox.SetCRISandbox(
		sbox.ID(),
		map[string]string{},
		map[string]string{},
		&types.PodSandboxMetadata{},
	); err != nil {
		t.Fatalf("SetCRISandbox: %v", err)
	}

	sbox.SetPrivileged(false)
	sbox.SetContainers(memorystore.New[*oci.Container]())

	sb, err := sbox.GetSandbox()
	if err != nil {
		t.Fatalf("GetSandbox: %v", err)
	}

	return sb
}

// TestNetworkStartSkipsCNIForIsolatedSandbox ensures networkStart never
// reaches the CNI plugin for isolated sandboxes. The server here has a nil
// config, so reaching SetUpPod would panic; instead bringUpLoopback must fail
// first on the empty test netns path, proving CNI was skipped.
func TestNetworkStartSkipsCNIForIsolatedSandbox(t *testing.T) {
	t.Parallel()

	sb := newNetworkIsolatedTestSandbox(t, true)
	sut := &Server{}

	_, _, err := sut.networkStart(context.Background(), sb)
	if err == nil {
		t.Fatal("networkStart() = nil, expected loopback error for empty netns path")
	}

	if !strings.Contains(err.Error(), "unable to set lo") {
		t.Errorf("networkStart() error = %q, expected loopback setup failure (CNI must be skipped)", err)
	}
}

// TestNetworkStopSkipsCNIForIsolatedSandbox ensures networkStop marks the
// sandbox stopped without invoking CNI teardown. A nil-config server would
// panic on TearDownPod, so a clean nil return proves the skip.
func TestNetworkStopSkipsCNIForIsolatedSandbox(t *testing.T) {
	t.Parallel()

	sb := newNetworkIsolatedTestSandbox(t, true)
	sut := &Server{}

	if err := sut.networkStop(context.Background(), sb); err != nil {
		t.Fatalf("networkStop() = %v, expected nil", err)
	}

	if !sb.NetworkStopped() {
		t.Error("networkStop() did not mark the sandbox network as stopped")
	}
}

// TestGetSandboxIPsIsolated ensures no IPs are reported for isolated
// sandboxes without consulting the CNI plugin.
func TestGetSandboxIPsIsolated(t *testing.T) {
	t.Parallel()

	sb := newNetworkIsolatedTestSandbox(t, true)
	sut := &Server{}

	ips, err := sut.getSandboxIPs(context.Background(), sb)
	if err != nil {
		t.Fatalf("getSandboxIPs() = %v, expected nil", err)
	}

	if len(ips) != 0 {
		t.Errorf("getSandboxIPs() = %v, expected no IPs", ips)
	}
}
