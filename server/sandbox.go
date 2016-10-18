package server

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/docker/docker/pkg/stringid"
	"github.com/kubernetes-incubator/cri-o/oci"
	"golang.org/x/sys/unix"
	"k8s.io/kubernetes/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

type sandboxNetNs struct {
	sync.Mutex
	ns       ns.NetNS
	symlink  *os.File
	closed   bool
	restored bool
}

func (ns *sandboxNetNs) symlinkCreate(name string) error {
	b := make([]byte, 4)
	_, randErr := rand.Reader.Read(b)
	if randErr != nil {
		return randErr
	}

	nsName := fmt.Sprintf("%s-%x", name, b)
	symlinkPath := filepath.Join(nsRunDir, nsName)

	if err := os.Symlink(ns.ns.Path(), symlinkPath); err != nil {
		return err
	}

	fd, err := os.Open(symlinkPath)
	if err != nil {
		if removeErr := os.RemoveAll(symlinkPath); removeErr != nil {
			return removeErr
		}

		return err
	}

	ns.symlink = fd

	return nil
}

func (ns *sandboxNetNs) symlinkRemove() error {
	if err := ns.symlink.Close(); err != nil {
		return err
	}

	return os.RemoveAll(ns.symlink.Name())
}

func isSymbolicLink(path string) (bool, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return false, err
	}

	return fi.Mode()&os.ModeSymlink == os.ModeSymlink, nil
}

func netNsGet(nspath, name string) (*sandboxNetNs, error) {
	if err := ns.IsNSorErr(nspath); err != nil {
		return nil, errSandboxClosedNetNS
	}

	symlink, symlinkErr := isSymbolicLink(nspath)
	if symlinkErr != nil {
		return nil, symlinkErr
	}

	var resolvedNsPath string
	if symlink {
		path, err := os.Readlink(nspath)
		if err != nil {
			return nil, err
		}
		resolvedNsPath = path
	} else {
		resolvedNsPath = nspath
	}

	netNS, err := ns.GetNS(resolvedNsPath)
	if err != nil {
		return nil, err
	}

	netNs := &sandboxNetNs{ns: netNS, closed: false, restored: true}

	if symlink {
		fd, err := os.Open(nspath)
		if err != nil {
			return nil, err
		}

		netNs.symlink = fd
	} else {
		if err := netNs.symlinkCreate(name); err != nil {
			return nil, err
		}
	}

	return netNs, nil
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
	cgroupParent   string
}

const (
	podDefaultNamespace = "default"
	defaultShmSize      = 64 * 1024 * 1024
	nsRunDir            = "/var/run/netns"
	podInfraCommand     = "/pause"
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

	return s.netns.symlink.Name()
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
		ns:     netNS,
		closed: false,
	}

	if err := s.netns.symlinkCreate(s.name); err != nil {
		logrus.Warnf("Could not create nentns symlink %v", err)

		if err := s.netns.ns.Close(); err != nil {
			return err
		}

		return err
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

	if err := s.netns.symlinkRemove(); err != nil {
		return err
	}

	if err := s.netns.ns.Close(); err != nil {
		return err
	}

	if s.netns.restored {
		if err := unix.Unmount(s.netns.ns.Path(), unix.MNT_DETACH); err != nil {
			return err
		}

		if err := os.RemoveAll(s.netns.ns.Path()); err != nil {
			return err
		}
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
		return nil, fmt.Errorf("specified pod sandbox not found: %s", sandboxID)
	}
	return sb, nil
}
