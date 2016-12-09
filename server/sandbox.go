package server

import (
	"errors"
	"fmt"

	"github.com/docker/docker/pkg/stringid"
	"github.com/kubernetes-incubator/cri-o/oci"
	"k8s.io/kubernetes/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

type sandbox struct {
	id             string
	name           string
	logDir         string
	labels         fields.Set
	annotations    map[string]string
	infraContainer *oci.Container
	containers     oci.Store
	processLabel   string
	mountLabel     string
	metadata       *pb.PodSandboxMetadata
	shmPath        string
}

const (
	podDefaultNamespace = "default"
	defaultShmSize      = 64 * 1024 * 1024
)

var (
	errSandboxIDEmpty = errors.New("PodSandboxId should not be empty")
)

func (s *sandbox) addContainer(c *oci.Container) {
	s.containers.Add(c.Name(), c)
}

func (s *sandbox) getContainer(name string) *oci.Container {
	return s.containers.Get(name)
}

func (s *sandbox) removeContainer(c *oci.Container) {
	s.containers.Delete(c.Name())
}

func (s *Server) generatePodIDandName(name string, namespace string, attempt uint32) (string, string, error) {
	var (
		err error
		id  = stringid.GenerateNonCryptoID()
	)
	if namespace == "" {
		namespace = podDefaultNamespace
	}

	if name, err = s.reservePodName(id, fmt.Sprintf("%s-%s-%v", namespace, name, attempt)); err != nil {
		return "", "", err
	}
	return id, name, err
}

type podSandboxRequest interface {
	GetPodSandboxId() string
}

func (s *Server) getPodSandboxFromRequest(req podSandboxRequest) (*sandbox, error) {
	sbID := req.GetPodSandboxId()
	if sbID == "" {
		return nil, errSandboxIDEmpty
	}

	sandboxID, err := s.podIDIndex.Get(sbID)
	if err != nil {
		return nil, fmt.Errorf("PodSandbox with ID starting with %s not found: %v", sbID, err)
	}

	sb := s.getSandbox(sandboxID)
	if sb == nil {
		return nil, fmt.Errorf("specified sandbox not found: %s", sandboxID)
	}
	return sb, nil
}
