//go:build skip_pod_runtime
// +build skip_pod_runtime

package oci

import (
	"errors"

	"github.com/cri-o/cri-o/pkg/config"
)

// newRuntimePod creates a new runtimePod instance
func newRuntimePod(*Runtime, *config.RuntimeHandler, *Container) (RuntimeImpl, error) {
	return nil, errors.New("unimplemented")
}
