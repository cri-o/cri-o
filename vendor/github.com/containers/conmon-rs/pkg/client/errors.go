package client

import "errors"

var (
	// ErrMissingIDMappings gets returned if user namespace unsharing is selected
	// but no IDMappings being provided.
	ErrMissingIDMappings = errors.New("unsharing user namespace selected but no IDMappings provided")

	// ErrUnsupported gets returned if the server does not support the feature.
	ErrUnsupported = errors.New("feature not supported by this conmon-rs version")

	// ErrCgroupManagerPerCommand gets returned if a per command configuration
	// asks for CgroupManagerPerCommand itself, which the server knows nothing
	// about.
	ErrCgroupManagerPerCommand = errors.New("the per command cgroup manager cannot be CgroupManagerPerCommand itself")
)
