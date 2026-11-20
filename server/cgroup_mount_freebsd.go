package server

import (
	rspec "github.com/opencontainers/runtime-spec/specs-go"
)

func preserveHostCgroupMountOptions(
	spec *rspec.Spec,
	readHostOptions func() ([]string, error),
) error {
	return nil
}
