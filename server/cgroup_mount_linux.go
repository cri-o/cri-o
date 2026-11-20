package server

import (
	"fmt"
	"slices"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
)

// The kernel applies the options of a cgroup2 mount made from the host cgroup
// namespace to the shared superblock. A mount without the host's options
// removes them from the host.
func preserveHostCgroupMountOptions(
	spec *rspec.Spec,
	readHostOptions func() ([]string, error),
) error {
	if hasNamespace(spec, rspec.CgroupNamespace) {
		return nil
	}

	i := slices.IndexFunc(spec.Mounts, func(m rspec.Mount) bool {
		return m.Destination == cgroupSysFsPath && m.Type == "cgroup"
	})
	if i < 0 {
		return nil
	}

	hostOptions, err := readHostOptions()
	if err != nil {
		return fmt.Errorf("read host cgroup mount options: %w", err)
	}

	mount := &spec.Mounts[i]
	for _, option := range hostOptions {
		if !slices.Contains(mount.Options, option) {
			mount.Options = append(mount.Options, option)
		}
	}

	return nil
}
