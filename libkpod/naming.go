package libkpod

import (
	"fmt"
	"strings"

	"github.com/docker/docker/pkg/stringid"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

const (
	// KubePrefix is the name prefix for kubernetes pods
	KubePrefix = "k8s"
	// InfraName is used to identify infra containers
	InfraName = "infra"
	// NameDelimiter is the delimeter between parts of a name
	NameDelimiter = "_"
)

func makeContainerName(sandboxMetadata *pb.PodSandboxMetadata, containerConfig *pb.ContainerConfig) string {
	return strings.Join([]string{
		KubePrefix,
		containerConfig.Metadata.Name,
		sandboxMetadata.Name,
		sandboxMetadata.Namespace,
		sandboxMetadata.Uid,
		fmt.Sprintf("%d", containerConfig.Metadata.Attempt),
	}, NameDelimiter)
}

func (c *ContainerServer) generateContainerIDandName(sandboxMetadata *pb.PodSandboxMetadata, containerConfig *pb.ContainerConfig) (string, string, error) {
	var (
		err error
		id  = stringid.GenerateNonCryptoID()
	)
	name, err := c.ReserveContainerName(id, makeContainerName(sandboxMetadata, containerConfig))
	if err != nil {
		return "", "", err
	}
	return id, name, err
}
