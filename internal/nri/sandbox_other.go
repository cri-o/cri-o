//go:build !linux

package nri

import (
	nri "github.com/containerd/nri/pkg/adaptation"
)

func podSandboxToNRI(pod PodSandbox) *nri.PodSandbox {
	return commonPodSandboxToNRI(pod)
}

func createUpdatePodSandboxRequest(pod PodSandbox) *nri.UpdatePodSandboxRequest {
	podNri := commonPodSandboxToNRI(pod)

	return &nri.UpdatePodSandboxRequest{
		Pod: podNri,
	}
}
