package server

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func TestValidateCgroupMountModeNamespaces(t *testing.T) {
	for _, tc := range []struct {
		name          string
		mode          types.CgroupMountMode
		hostNet       bool
		usernsEnabled bool
		wantErr       bool
	}{
		{
			name:          "read-write in host network and a user namespace is rejected",
			mode:          types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_WRITE,
			hostNet:       true,
			usernsEnabled: true,
			wantErr:       true,
		},
		{
			name:          "read-only in host network and a user namespace is accepted",
			mode:          types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_ONLY,
			hostNet:       true,
			usernsEnabled: true,
		},
		{
			name:          "unspecified in host network and a user namespace is accepted",
			mode:          types.CgroupMountMode_CGROUP_MOUNT_MODE_UNSPECIFIED,
			hostNet:       true,
			usernsEnabled: true,
		},
		{
			name:    "read-write in host network without a user namespace is accepted",
			mode:    types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_WRITE,
			hostNet: true,
		},
		{
			name:          "read-write in a user namespace without host network is accepted",
			mode:          types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_WRITE,
			usernsEnabled: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &types.ContainerConfig{Linux: &types.LinuxContainerConfig{
				SecurityContext: &types.LinuxContainerSecurityContext{CgroupMountMode: tc.mode},
			}}

			err := validateCgroupMountModeNamespaces(config, tc.hostNet, tc.usernsEnabled)
			if (err != nil) != tc.wantErr {
				t.Fatalf("got error %v, want error %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateCgroupMountNotCovered(t *testing.T) {
	for _, tc := range []struct {
		name             string
		mode             types.CgroupMountMode
		containerPath    string
		imageVolumePath  string
		wantCoveringPath string
	}{
		{
			name:          "unspecified lets a mount replace the cgroup mount",
			mode:          types.CgroupMountMode_CGROUP_MOUNT_MODE_UNSPECIFIED,
			containerPath: "/sys/fs/cgroup",
		},
		{
			name:             "read-only rejects a mount at /sys/fs/cgroup",
			mode:             types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_ONLY,
			containerPath:    "/sys/fs/cgroup",
			wantCoveringPath: "/sys/fs/cgroup",
		},
		{
			name:             "read-write rejects a mount at /sys/fs",
			mode:             types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_WRITE,
			containerPath:    "/sys/fs",
			wantCoveringPath: "/sys/fs",
		},
		{
			name:             "read-write rejects an uncleaned mount at /sys",
			mode:             types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_WRITE,
			containerPath:    "/sys/",
			wantCoveringPath: "/sys",
		},
		{
			name:             "read-only rejects a mount at the root",
			mode:             types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_ONLY,
			containerPath:    "/",
			wantCoveringPath: "/",
		},
		{
			name:             "read-only rejects an image volume at /sys/fs/cgroup",
			mode:             types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_ONLY,
			imageVolumePath:  "/sys/fs/cgroup",
			wantCoveringPath: "/sys/fs/cgroup",
		},
		{
			name:             "read-write rejects an image volume at /sys",
			mode:             types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_WRITE,
			imageVolumePath:  "/sys",
			wantCoveringPath: "/sys",
		},
		{
			name:          "read-write allows a mount below /sys/fs/cgroup",
			mode:          types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_WRITE,
			containerPath: "/sys/fs/cgroup/foo",
		},
		{
			name:          "read-write allows a mount at a sibling of /sys",
			mode:          types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_WRITE,
			containerPath: "/sysroot",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &types.ContainerConfig{Linux: &types.LinuxContainerConfig{
				SecurityContext: &types.LinuxContainerSecurityContext{CgroupMountMode: tc.mode},
			}}
			if tc.containerPath != "" {
				config.Mounts = []*types.Mount{{HostPath: "/host", ContainerPath: tc.containerPath}}
			}

			var imageVolumeMounts []rspec.Mount
			if tc.imageVolumePath != "" {
				imageVolumeMounts = []rspec.Mount{
					{Source: "/volume", Destination: tc.imageVolumePath, Type: "bind"},
				}
			}

			err := validateCgroupMountNotCovered(config, imageVolumeMounts)

			if tc.wantCoveringPath == "" {
				if err != nil {
					t.Fatalf("got error %v, want nil", err)
				}

				return
			}

			want := fmt.Sprintf("the mount at %q covers /sys/fs/cgroup", tc.wantCoveringPath)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("got error %v, want an error containing %q", err, want)
			}
		})
	}
}

func TestApplyCgroupMountMode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mode  types.CgroupMountMode
		mount rspec.Mount
		want  []string
	}{
		{
			name:  "unspecified keeps the default",
			mode:  types.CgroupMountMode_CGROUP_MOUNT_MODE_UNSPECIFIED,
			mount: cgroupMount("rw"),
			want:  []string{"nosuid", "noexec", "nodev", "relatime", "rw"},
		},
		{
			name:  "read-write overrides the read-only default",
			mode:  types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_WRITE,
			mount: cgroupMount("ro"),
			want:  []string{"nosuid", "noexec", "nodev", "relatime", "rw"},
		},
		{
			name:  "read-only overrides the annotation",
			mode:  types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_ONLY,
			mount: cgroupMount("rw"),
			want:  []string{"nosuid", "noexec", "nodev", "relatime", "ro"},
		},
		{
			name: "read-only overrides the systemd mount",
			mode: types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_ONLY,
			mount: rspec.Mount{
				Destination: cgroupSysFsPath,
				Type:        "cgroup",
				Source:      "cgroup",
				Options:     []string{"private", "rw"},
			},
			want: []string{"private", "ro"},
		},
		{
			name: "supplied bind is unchanged",
			mode: types.CgroupMountMode_CGROUP_MOUNT_MODE_READ_ONLY,
			mount: rspec.Mount{
				Destination: cgroupSysFsPath,
				Type:        "bind",
				Source:      cgroupSysFsPath,
				Options:     []string{"rbind", "rw"},
			},
			want: []string{"rbind", "rw"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, err := generate.New("linux")
			if err != nil {
				t.Fatal(err)
			}

			g.RemoveMount(cgroupSysFsPath)
			g.AddMount(tc.mount)

			applyCgroupMountMode(&g, &types.ContainerConfig{Linux: &types.LinuxContainerConfig{
				SecurityContext: &types.LinuxContainerSecurityContext{CgroupMountMode: tc.mode},
			}})

			i := slices.IndexFunc(g.Config.Mounts, func(m rspec.Mount) bool {
				return m.Destination == cgroupSysFsPath
			})
			if got := g.Config.Mounts[i].Options; !slices.Equal(got, tc.want) {
				t.Fatalf("got options %v, want %v", got, tc.want)
			}
		})
	}
}
