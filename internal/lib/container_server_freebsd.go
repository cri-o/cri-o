package lib

import (
	"fmt"

	rspec "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/cri-o/cri-o/pkg/annotations"
)

func configNsPath(spec *rspec.Spec, nsType rspec.LinuxNamespaceType) (string, error) {
	if nsType == rspec.NetworkNamespace {
		// On FreeBSD, if we are not using HostNetwork, the namespace
		// 'path' is the sandbox ID which is used as the name for the
		// infra container jail which owns the pod vnet.
		if !isTrue(spec.Annotations[annotations.HostNetwork]) {
			return spec.Annotations[annotations.SandboxID], nil
		}
	}
	return "", fmt.Errorf("missing networking namespace")
}
