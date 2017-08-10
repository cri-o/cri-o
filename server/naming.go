package server

import (
	"fmt"
	"strings"

	"github.com/docker/docker/pkg/stringid"
	"github.com/kubernetes-incubator/cri-o/libkpod"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

func makeSandboxName(sandboxConfig *pb.PodSandboxConfig) string {
	return strings.Join([]string{
		libkpod.KubePrefix,
		sandboxConfig.Metadata.Name,
		sandboxConfig.Metadata.Namespace,
		sandboxConfig.Metadata.Uid,
		fmt.Sprintf("%d", sandboxConfig.Metadata.Attempt),
	}, libkpod.NameDelimiter)
}

func makeSandboxContainerName(sandboxConfig *pb.PodSandboxConfig) string {
	return strings.Join([]string{
		libkpod.KubePrefix,
		libkpod.InfraName,
		sandboxConfig.Metadata.Name,
		sandboxConfig.Metadata.Namespace,
		sandboxConfig.Metadata.Uid,
		fmt.Sprintf("%d", sandboxConfig.Metadata.Attempt),
	}, libkpod.NameDelimiter)
}

func (s *Server) generatePodIDandName(sandboxConfig *pb.PodSandboxConfig) (string, string, error) {
	var (
		err error
		id  = stringid.GenerateNonCryptoID()
	)
	if sandboxConfig.Metadata.Namespace == "" {
		return "", "", fmt.Errorf("cannot generate pod ID without namespace")
	}
	name, err := s.ReservePodName(id, makeSandboxName(sandboxConfig))
	if err != nil {
		return "", "", err
	}
	return id, name, err
}

func (s *Server) generateContainerIDandNameForSandbox(sandboxConfig *pb.PodSandboxConfig) (string, string, error) {
	var (
		err error
		id  = stringid.GenerateNonCryptoID()
	)
	name, err := s.ReserveContainerName(id, makeSandboxContainerName(sandboxConfig))
	if err != nil {
		return "", "", err
	}
	return id, name, err
}
