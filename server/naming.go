package server

import (
	"fmt"
	"strings"

	"github.com/containers/storage/pkg/stringid"
	"github.com/cri-o/cri-o/server/cri/types"
)

const (
	kubePrefix    = "k8s"
	nameDelimiter = "_"
)

func makeSandboxContainerName(sandboxConfig *types.PodSandboxConfig) string {
	return strings.Join([]string{
		kubePrefix,
		types.InfraContainerName,
		sandboxConfig.Metadata.Name,
		sandboxConfig.Metadata.Namespace,
		sandboxConfig.Metadata.UID,
		fmt.Sprintf("%d", sandboxConfig.Metadata.Attempt),
	}, nameDelimiter)
}

func (s *Server) ReserveSandboxContainerIDAndName(config *types.PodSandboxConfig) (string, error) {
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
