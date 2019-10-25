package server

import (
	"fmt"
	"strings"

	"github.com/containers/storage/pkg/stringid"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	kubePrefix    = "k8s"
	infraName     = "POD"
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

func (s *Server) ReservePodIDAndName(config *pb.PodSandboxConfig) (id, name string, err error) {
	if config == nil || config.Metadata == nil || config.Metadata.Namespace == "" {
		return "", "", fmt.Errorf("cannot generate pod name without namespace")
	}

	id = stringid.GenerateNonCryptoID()
	name, err = s.ReservePodName(id, makeSandboxName(config))

	if err != nil {
		return "", "", err
	}
	return id, name, nil
}

func (s *Server) ReserveSandboxContainerIDAndName(config *pb.PodSandboxConfig) (name string, err error) {
	if config == nil || config.Metadata == nil {
		return "", fmt.Errorf("cannot generate sandbox container name without metadata")
	}

	id := stringid.GenerateNonCryptoID()
	name, err = s.ReserveContainerName(id, makeSandboxContainerName(config))
	if err != nil {
		return "", err
	}
	return name, err
}

func (s *Server) ReserveContainerIDandName(sandboxMetadata *pb.PodSandboxMetadata, config *pb.ContainerConfig) (id, name string, err error) {
	if config == nil || config.Metadata == nil || sandboxMetadata == nil {
		return "", "", fmt.Errorf("cannot generate container name without metadata")
	}

	id = stringid.GenerateNonCryptoID()
	name, err = s.ReserveContainerName(id, makeContainerName(sandboxMetadata, config))
	if err != nil {
		return "", "", err
	}
	return id, name, err
}
