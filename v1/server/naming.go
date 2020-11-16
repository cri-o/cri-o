package server

import (
	"fmt"
	"strings"

	"github.com/containers/storage/pkg/stringid"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	kubePrefix    = "k8s"
	infraName     = "POD"
	nameDelimiter = "_"
)

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

func (s *Server) ReserveSandboxContainerIDAndName(config *pb.PodSandboxConfig) (string, error) {
	if config == nil || config.Metadata == nil {
		return "", fmt.Errorf("cannot generate sandbox container name without metadata")
	}

	id := stringid.GenerateNonCryptoID()
	name, err := s.ReserveContainerName(id, makeSandboxContainerName(config))
	if err != nil {
		return "", err
	}
	return name, err
}
