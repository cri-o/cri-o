package server

import (
	"slices"
	"testing"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

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
