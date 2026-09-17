package server

import rspec "github.com/opencontainers/runtime-spec/specs-go"

func cgroupMount(accessOption string) rspec.Mount {
	return rspec.Mount{
		Destination: cgroupSysFsPath,
		Type:        "cgroup",
		Source:      "cgroup",
		Options:     []string{"nosuid", "noexec", "nodev", "relatime", accessOption},
	}
}
