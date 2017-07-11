package server

import (
	"fmt"
	"strings"

	"github.com/docker/docker/pkg/stringid"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

const (
	kubePrefix    = "k8s"
	infraName     = "infra"
	nameDelimiter = "_"
)

func makeSandboxName(sandboxConfig *pb.PodSandboxConfig) string {
	return strings.Join([]string{
		kubePrefix,
		sandboxConfig.Metadata.Name,
		sandboxConfig.Metadata.Namespace,
		sandboxConfig.Metadata.Uid,
		fmt.Sprintf("%d", sandboxConfig.Metadata.Attempt),
	}, nameDelimiter)
}

func makeSandboxContainerName(sandboxConfig *pb.PodSandboxConfig) string {
	return strings.Join([]string{
		kubePrefix,
		infraName,
		sandboxConfig.Metadata.Name,
		sandboxConfig.Metadata.Namespace,
		sandboxConfig.Metadata.Uid,
		fmt.Sprintf("%d", sandboxConfig.Metadata.Attempt),
	}, nameDelimiter)
}

func makeContainerName(sandboxMetadata *pb.PodSandboxMetadata, containerConfig *pb.ContainerConfig) string {
	return strings.Join([]string{
		kubePrefix,
		containerConfig.Metadata.Name,
		sandboxMetadata.Name,
		sandboxMetadata.Namespace,
		sandboxMetadata.Uid,
		fmt.Sprintf("%d", containerConfig.Metadata.Attempt),
	}, nameDelimiter)
}

func (s *Server) generatePodIDandName(sandboxConfig *pb.PodSandboxConfig) (string, string, error) {
	var (
		err  error
		id   = stringid.GenerateNonCryptoID()
		name = makeSandboxName(sandboxConfig)
	)
	if sandboxConfig.Metadata.Namespace == "" {
		return "", "", fmt.Errorf("cannot generate pod ID without namespace")
	}
	return id, name, err
}

func (s *Server) generateContainerIDandNameForSandbox(sandboxConfig *pb.PodSandboxConfig) (string, string, error) {
	var (
		err  error
		id   = stringid.GenerateNonCryptoID()
		name = makeSandboxContainerName(sandboxConfig)
	)
	return id, name, err
}

func (s *Server) generateContainerIDandName(sandboxMetadata *pb.PodSandboxMetadata, containerConfig *pb.ContainerConfig) (string, string, error) {
	var (
		err  error
		id   = stringid.GenerateNonCryptoID()
		name = makeContainerName(sandboxMetadata, containerConfig)
	)
	return id, name, err
}
