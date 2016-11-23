package server

import (
	"errors"
	"fmt"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/stringid"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/containernetworking/cni/pkg/ns"
	"k8s.io/kubernetes/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

type sandboxNetNs struct {
	sync.Mutex
	ns     ns.NetNS
	closed bool
}

func netNsGet(nspath string) (*sandboxNetNs, error) {
	if err := ns.IsNSorErr(nspath); err != nil {
		return nil, errSandboxClosedNetNS
	}

	netNS, err := ns.GetNS(nspath)
	if err != nil {
		return nil, err
	}

	return &sandboxNetNs{ns: netNS, closed: false,}, nil
}

func hostNetNsPath() (string, error) {
	netNS, err := ns.GetCurrentNS()
	if err != nil {
		return "", err
	}

	defer netNS.Close()

	return netNS.Path(), nil
}

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
	netns          *sandboxNetNs
	metadata       *pb.PodSandboxMetadata
	shmPath        string
}

const (
	podDefaultNamespace = "default"
	defaultShmSize      = 64 * 1024 * 1024
)

var (
	errSandboxIDEmpty     = errors.New("PodSandboxId should not be empty")
	errSandboxClosedNetNS = errors.New("PodSandbox networking namespace is closed")
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

func (s *sandbox) netNs() ns.NetNS {
	if s.netns == nil {
		return nil
	}

	return s.netns.ns
}

func (s *sandbox) netNsPath() string {
	if s.netns == nil {
		return ""
	}

	return s.netns.ns.Path()
}

func (s *sandbox) netNsCreate() error {
	if s.netns != nil {
		return fmt.Errorf("net NS already created")
	}

	netNS, err := ns.NewNS()
	if err != nil {
		return err
	}

	s.netns = &sandboxNetNs{
		ns: netNS,
		closed: false,
	}

	return nil
}

func (s *sandbox) netNsRemove() error {
	if s.netns == nil {
		logrus.Warn("no networking namespace")
		return nil
	}

	s.netns.Lock()
	defer s.netns.Unlock()

	if s.netns.closed {
		// netNsRemove() can be called multiple
		// times without returning an error.
		return nil
	}

	if err := s.netns.ns.Close(); err != nil {
		return err
	}

	s.netns.closed = true
	return nil
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
