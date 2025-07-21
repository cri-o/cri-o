package server

import (
	"errors"
	"strconv"
	"strings"

	"github.com/containers/storage/pkg/stringid"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
)

const (
	kubePrefix    = "k8s"
	nameDelimiter = "_"
)

func makeSandboxContainerName(sandboxConfig *types.PodSandboxConfig) string {
	return strings.Join([]string{
		kubePrefix,
		oci.InfraContainerName,
		sandboxConfig.GetMetadata().GetName(),
		sandboxConfig.GetMetadata().GetNamespace(),
		sandboxConfig.GetMetadata().GetUid(),
		strconv.FormatUint(uint64(sandboxConfig.GetMetadata().GetAttempt()), 10),
	}, nameDelimiter)
}

func (s *Server) ReserveSandboxContainerIDAndName(config *types.PodSandboxConfig) (string, error) {
	if config == nil || config.GetMetadata() == nil {
		return "", errors.New("cannot generate sandbox container name without metadata")
	}

	id := stringid.GenerateNonCryptoID()

	name, err := s.ReserveContainerName(id, makeSandboxContainerName(config))
	if err != nil {
		return "", err
	}

	return name, err
}
