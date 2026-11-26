package server

import (
	"slices"
	"testing"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/factory/container"
)

func TestResolveCgroupnsMode(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		desc         string
		privileged   bool
		cgroupnsOpts *types.CgroupNamespace
		cgroupV2     bool
		userns       bool
		want         types.NamespaceMode
		wantErr      bool
	}{
		{desc: "default: non-privileged on cgroup v2", cgroupV2: true, want: types.NamespaceMode_CONTAINER},
		{desc: "default: privileged on cgroup v2", privileged: true, cgroupV2: true, want: types.NamespaceMode_NODE},
		{desc: "default: non-privileged on cgroup v1", want: types.NamespaceMode_NODE},
		{desc: "default: privileged on cgroup v1", privileged: true, want: types.NamespaceMode_NODE},
		{
			desc:         "CONTAINER: privileged on cgroup v2",
			privileged:   true,
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_CONTAINER},
			cgroupV2:     true,
			want:         types.NamespaceMode_CONTAINER,
		},
		{
			desc:         "CONTAINER: non-privileged on cgroup v1",
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_CONTAINER},
			want:         types.NamespaceMode_CONTAINER,
		},
		{
			desc:         "NODE: non-privileged on cgroup v2",
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_NODE},
			cgroupV2:     true,
			want:         types.NamespaceMode_NODE,
		},
		{
			desc:         "NODE: with user namespaces",
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_NODE},
			cgroupV2:     true,
			userns:       true,
			wantErr:      true,
		},
		{
			desc:         "CONTAINER: with user namespaces",
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_CONTAINER},
			cgroupV2:     true,
			userns:       true,
			want:         types.NamespaceMode_CONTAINER,
		},
		{
			desc:       "default: privileged with user namespaces",
			privileged: true,
			cgroupV2:   true,
			userns:     true,
			want:       types.NamespaceMode_NODE,
		},
		{
			desc:         "POD is unsupported",
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_POD},
			cgroupV2:     true,
			wantErr:      true,
		},
		{
			desc:         "TARGET is unsupported",
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_TARGET},
			cgroupV2:     true,
			wantErr:      true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			got, err := resolveCgroupnsMode(tc.cgroupnsOpts, tc.privileged, tc.cgroupV2, tc.userns)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSetupCgroupNamespace must not run in parallel, as node.CgroupIsV2()
// is not safe for concurrent use.
func TestSetupCgroupNamespace(t *testing.T) {
	for _, tc := range []struct {
		desc           string
		privileged     bool
		cgroupnsOpts   *types.CgroupNamespace
		cgroupWritable bool
		mountMode      types.CgroupMountMode
		usernsOpts     *types.UserNamespace
		// usernsEnabled denotes that the sandbox has a user namespace.
		usernsEnabled bool
		// inherited denotes that the spec already has a cgroup namespace.
		inherited bool
		// wantCgroupns is nil when the expectation depends on the host cgroup version.
		wantCgroupns *bool
		wantErr      bool
	}{
		{
			desc:         "default: non-privileged",
			wantCgroupns: nil,
		},
		{
			desc:         "default: privileged keeps the spec as is",
			privileged:   true,
			inherited:    true,
			wantCgroupns: new(true),
		},
		{
			desc:         "CONTAINER: privileged",
			privileged:   true,
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_CONTAINER},
			wantCgroupns: new(true),
		},
		{
			desc:         "NODE: non-privileged",
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_NODE},
			wantCgroupns: new(false),
		},
		{
			desc:         "NODE: inherited cgroup namespace is removed",
			privileged:   true,
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_NODE},
			inherited:    true,
			wantCgroupns: new(false),
		},
		{
			desc:           "NODE: writable cgroup, non-privileged",
			cgroupnsOpts:   &types.CgroupNamespace{Mode: types.NamespaceMode_NODE},
			cgroupWritable: true,
			wantErr:        true,
		},
		{
			desc:           "NODE: writable cgroup, privileged",
			privileged:     true,
			cgroupnsOpts:   &types.CgroupNamespace{Mode: types.NamespaceMode_NODE},
			cgroupWritable: true,
			wantCgroupns:   new(false),
		},
		{
			desc:           "CONTAINER: writable cgroup, non-privileged",
			cgroupnsOpts:   &types.CgroupNamespace{Mode: types.NamespaceMode_CONTAINER},
			cgroupWritable: true,
			wantCgroupns:   new(true),
		},
		{
			desc:         "NODE: READ_WRITE mount mode, non-privileged",
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_NODE},
			mountMode:    types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_WRITE,
			wantErr:      true,
		},
		{
			desc:         "NODE: READ_WRITE mount mode, privileged",
			privileged:   true,
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_NODE},
			mountMode:    types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_WRITE,
			wantCgroupns: new(false),
		},
		{
			desc:         "NODE: READ_ONLY mount mode, non-privileged",
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_NODE},
			mountMode:    types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_ONLY,
			wantCgroupns: new(false),
		},
		{
			desc:         "CONTAINER: READ_WRITE mount mode, non-privileged",
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_CONTAINER},
			mountMode:    types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_WRITE,
			wantCgroupns: new(true),
		},
		{
			desc:         "NODE: POD user namespace mode",
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_NODE},
			usernsOpts:   &types.UserNamespace{Mode: types.NamespaceMode_POD},
			wantErr:      true,
		},
		{
			desc:          "NODE: sandbox with user namespace",
			cgroupnsOpts:  &types.CgroupNamespace{Mode: types.NamespaceMode_NODE},
			usernsEnabled: true,
			wantErr:       true,
		},
		{
			desc:         "NODE: NODE user namespace mode",
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_NODE},
			usernsOpts:   &types.UserNamespace{Mode: types.NamespaceMode_NODE},
			wantCgroupns: new(false),
		},
		{
			desc:         "CONTAINER: POD user namespace mode",
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_CONTAINER},
			usernsOpts:   &types.UserNamespace{Mode: types.NamespaceMode_POD},
			wantCgroupns: new(true),
		},
		{
			desc:         "POD is rejected",
			cgroupnsOpts: &types.CgroupNamespace{Mode: types.NamespaceMode_POD},
			wantErr:      true,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ctr, err := container.New()
			if err != nil {
				t.Fatal(err)
			}

			if err := ctr.SetConfig(&types.ContainerConfig{
				Metadata: &types.ContainerMetadata{Name: "testctr"},
				Linux: &types.LinuxContainerConfig{
					SecurityContext: &types.LinuxContainerSecurityContext{
						Privileged:      tc.privileged,
						CgroupMountMode: tc.mountMode,
						NamespaceOptions: &types.NamespaceOption{
							CgroupnsOptions: tc.cgroupnsOpts,
							UsernsOptions:   tc.usernsOpts,
						},
					},
				},
			}, &types.PodSandboxConfig{
				Metadata: &types.PodSandboxMetadata{Name: "testpod"},
				Linux: &types.LinuxPodSandboxConfig{
					SecurityContext: &types.LinuxSandboxSecurityContext{Privileged: tc.privileged},
				},
			}); err != nil {
				t.Fatal(err)
			}

			if err := ctr.SetPrivileged(); err != nil {
				t.Fatal(err)
			}

			specgen := ctr.Spec()
			if tc.inherited {
				if err := specgen.AddOrReplaceLinuxNamespace(
					string(rspec.CgroupNamespace),
					"",
				); err != nil {
					t.Fatal(err)
				}
			}

			err = (&Server{}).setupCgroupNamespace(
				ctr,
				specgen,
				tc.cgroupWritable,
				tc.usernsEnabled,
			)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			want := node.CgroupIsV2()
			if tc.wantCgroupns != nil {
				want = *tc.wantCgroupns
			}

			got := slices.ContainsFunc(
				specgen.Config.Linux.Namespaces,
				func(ns rspec.LinuxNamespace) bool {
					return ns.Type == rspec.CgroupNamespace
				},
			)
			if got != want {
				t.Errorf("cgroup namespace: got %v, want %v", got, want)
			}
		})
	}
}
