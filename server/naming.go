package server

import (
	"errors"
	"strconv"
	"strings"

	"github.com/containers/storage/pkg/stringid"
	"github.com/cri-o/cri-o/internal/oci"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	kubePrefix    = "k8s"
	nameDelimiter = "_"
)

func makeSandboxContainerName(sandboxConfig *types.PodSandboxConfig) string {
	return strings.Join([]string{
		kubePrefix,
		oci.InfraContainerName,
		sandboxConfig.Metadata.Name,
		sandboxConfig.Metadata.Namespace,
		sandboxConfig.Metadata.Uid,
		strconv.FormatUint(uint64(sandboxConfig.Metadata.Attempt), 10),
	}, nameDelimiter)
}

func (s *Server) ReserveSandboxContainerIDAndName(config *types.PodSandboxConfig) (string, error) {
	if config == nil || config.Metadata == nil {
		return "", errors.New("cannot generate sandbox container name without metadata")
	}

	id := stringid.GenerateNonCryptoID()
	name, err := s.ReserveContainerName(id, makeSandboxContainerName(config))
	if err != nil {
		return "", err
	}
	return name, err
}
