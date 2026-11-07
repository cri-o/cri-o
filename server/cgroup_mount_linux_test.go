package server

import (
	"errors"
	"slices"
	"testing"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
)

func TestPreserveHostCgroupMountOptions(t *testing.T) {
	t.Parallel()

	hostOptions := []string{"nsdelegate", "memory_recursiveprot"}
	cgroupMount := rspec.Mount{
		Destination: cgroupSysFsPath,
		Type:        "cgroup",
		Source:      "cgroup",
		Options:     []string{"nosuid", "rw"},
	}
	bindMount := rspec.Mount{
		Destination: cgroupSysFsPath,
		Type:        "bind",
		Source:      cgroupSysFsPath,
		Options:     []string{"rbind", "rw"},
	}
	cgroupNamespace := &rspec.Linux{
		Namespaces: []rspec.LinuxNamespace{{Type: rspec.CgroupNamespace}},
	}
	pidNamespace := &rspec.Linux{
		Namespaces: []rspec.LinuxNamespace{{Type: rspec.PIDNamespace}},
	}

	for _, tc := range []struct {
		name            string
		spec            rspec.Spec
		readHostOptions func() ([]string, error)
		want            []string
		wantErr         bool
	}{
		{
			name:            "host cgroup namespace gets the host options",
			spec:            rspec.Spec{Linux: pidNamespace, Mounts: []rspec.Mount{cgroupMount}},
			readHostOptions: hostOptionsReader(hostOptions, nil),
			want:            []string{"nosuid", "rw", "nsdelegate", "memory_recursiveprot"},
		},
		{
			name:            "no namespaces gets the host options",
			spec:            rspec.Spec{Mounts: []rspec.Mount{cgroupMount}},
			readHostOptions: hostOptionsReader(hostOptions, nil),
			want:            []string{"nosuid", "rw", "nsdelegate", "memory_recursiveprot"},
		},
		{
			name:            "host options already on the mount are not repeated",
			spec:            rspec.Spec{Mounts: []rspec.Mount{cgroupMount}},
			readHostOptions: hostOptionsReader([]string{"nosuid", "nsdelegate"}, nil),
			want:            []string{"nosuid", "rw", "nsdelegate"},
		},
		{
			name:            "host options cannot be read",
			spec:            rspec.Spec{Mounts: []rspec.Mount{cgroupMount}},
			readHostOptions: hostOptionsReader(nil, errors.New("mountinfo")),
			wantErr:         true,
		},
		{
			name:            "cgroup namespace keeps the mount and does not read the host",
			spec:            rspec.Spec{Linux: cgroupNamespace, Mounts: []rspec.Mount{cgroupMount}},
			readHostOptions: hostOptionsReader(nil, errors.New("must not be called")),
			want:            []string{"nosuid", "rw"},
		},
		{
			name:            "bind mount keeps its options and does not read the host",
			spec:            rspec.Spec{Mounts: []rspec.Mount{bindMount}},
			readHostOptions: hostOptionsReader(nil, errors.New("must not be called")),
			want:            []string{"rbind", "rw"},
		},
		{
			name:            "no cgroup mount does not read the host",
			spec:            rspec.Spec{Mounts: []rspec.Mount{{Destination: "/sys", Type: "sysfs", Source: "sysfs"}}},
			readHostOptions: hostOptionsReader(nil, errors.New("must not be called")),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := preserveHostCgroupMountOptions(&tc.spec, tc.readHostOptions)
			if (err != nil) != tc.wantErr {
				t.Fatalf("got error %v, want error %t", err, tc.wantErr)
			}

			if tc.wantErr {
				return
			}

			if got := mountOptionsAt(&tc.spec, cgroupSysFsPath); !slices.Equal(got, tc.want) {
				t.Fatalf("got options %v, want %v", got, tc.want)
			}
		})
	}
}

func hostOptionsReader(options []string, err error) func() ([]string, error) {
	return func() ([]string, error) {
		return options, err
	}
}

func mountOptionsAt(spec *rspec.Spec, destination string) []string {
	for _, m := range spec.Mounts {
		if m.Destination == destination {
			return m.Options
		}
	}

	return nil
}
