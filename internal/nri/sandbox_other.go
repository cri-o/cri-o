//go:build !linux
// +build !linux

package nri

import (
	nri "github.com/containerd/nri/pkg/adaptation"
)

func podSandboxToNRI(pod PodSandbox) *nri.PodSandbox {
	return commonPodSandboxToNRI(pod)
}
