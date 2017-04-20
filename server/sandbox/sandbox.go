package sandbox

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/kubernetes-incubator/cri-o/oci"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/fields"
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
		return nil, ErrSandboxClosedNetNS
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

// HostNetNsPath returns the path of the host's network namespace
func HostNetNsPath() (string, error) {
	netNS, err := ns.GetCurrentNS()
	if err != nil {
		return "", err
	}

	defer netNS.Close()

	return netNS.Path(), nil
}

// Sandbox represents a single pod sandbox
type Sandbox struct {
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
	privileged     bool
	resolvPath     string
	hostname       string
}

const (
	// PodDefaultNamespace is the default namespace name for pods
	PodDefaultNamespace = "default"
	// DefaultShmSize is the default size of the SHM device for sandboxs
	DefaultShmSize = 64 * 1024 * 1024
	// PodInfraCommand is the default pause command for pods
	PodInfraCommand = "/pause"
	nsRunDir        = "/var/run/netns"
)

var (
	// ErrSandboxIDEmpty is the error returned when an operation passes "" instead of a sandbox ID
	ErrSandboxIDEmpty = errors.New("PodSandboxId should not be empty")
	// ErrSandboxClosedNetNS is the error returned when a network namespace is closed and cannot be joined
	ErrSandboxClosedNetNS = errors.New("PodSandbox networking namespace is closed")
)

// New creates and populates a new sandbox
// New sandboxes have no containers, no infra container, and no network namespace associated with them.
// An infra container must be attached before the sandbox is added to the state
func New(id, name, logDir string, labels, annotations map[string]string, processLabel, mountLabel string, metadata *pb.PodSandboxMetadata, shmPath, cgroupParent string, privileged bool, resolvPath, hostname string) (*Sandbox, error) {
	sb := new(Sandbox)
	sb.id = id
	sb.name = name
	sb.logDir = logDir
	sb.labels = labels
	sb.annotations = annotations
	sb.containers = oci.NewMemoryStore()
	sb.processLabel = processLabel
	sb.mountLabel = mountLabel
	sb.metadata = metadata
	sb.shmPath = shmPath
	sb.cgroupParent = cgroupParent
	sb.privileged = privileged
	sb.resolvPath = resolvPath
	sb.hostname = hostname

	return sb, nil
}

// ID returns the sandbox's ID
func (s *Sandbox) ID() string {
	return s.id
}

// Name returns the sandbox's name
func (s *Sandbox) Name() string {
	return s.name
}

// LogDir returns the directory the sandbox logs to
func (s *Sandbox) LogDir() string {
	return s.logDir
}

// Labels returns the sandbox's labels
func (s *Sandbox) Labels() map[string]string {
	return s.labels
}

// Annotations returns the sandbox's annotations
func (s *Sandbox) Annotations() map[string]string {
	return s.annotations
}

// InfraContainer returns the sandbox's infrastructure container
func (s *Sandbox) InfraContainer() *oci.Container {
	return s.infraContainer
}

// Containers returns an array of all the containers in the sandbox
func (s *Sandbox) Containers() []*oci.Container {
	return s.containers.List()
}

// ProcessLabel returns the SELinux process label of the sandbox
func (s *Sandbox) ProcessLabel() string {
	return s.processLabel
}

// MountLabel returns the SELinux mount label of the sandbox
func (s *Sandbox) MountLabel() string {
	return s.mountLabel
}

// Metadata returns Kubernetes metadata associated with the sandbox
func (s *Sandbox) Metadata() *pb.PodSandboxMetadata {
	return s.metadata
}

// ShmPath returns the path to the sandbox's shared memory device
func (s *Sandbox) ShmPath() string {
	return s.shmPath
}

// CgroupParent returns the sandbox's CGroup parent
func (s *Sandbox) CgroupParent() string {
	return s.cgroupParent
}

// Privileged returns whether the sandbox can support privileged containers
func (s *Sandbox) Privileged() bool {
	return s.privileged
}

// ResolvPath returns the path to the sandbox's DNS resolver configuration
func (s *Sandbox) ResolvPath() string {
	return s.resolvPath
}

// Hostname returns the sandbox's hostname
func (s *Sandbox) Hostname() string {
	return s.hostname
}

// AddContainer adds a container to the sandbox
func (s *Sandbox) AddContainer(c *oci.Container) {
	s.containers.Add(c.ID(), c)
}

// GetContainer retrieves the container with given ID from the sandbox
// Returns nil if no such container exists
func (s *Sandbox) GetContainer(id string) *oci.Container {
	return s.containers.Get(id)
}

// RemoveContainer removes the container with given ID from the sandbox
// If no container with that ID exists in the sandbox, no action is taken
func (s *Sandbox) RemoveContainer(id string) {
	s.containers.Delete(id)
}

// SetInfraContainer sets the infrastructure container of a sandbox
// Attempts to set the infrastructure container after one is already present will throw an error
func (s *Sandbox) SetInfraContainer(infraCtr *oci.Container) error {
	if s.infraContainer != nil {
		return fmt.Errorf("sandbox already has an infra container")
	} else if infraCtr == nil {
		return fmt.Errorf("must provide non-nil infra container")
	}

	s.infraContainer = infraCtr

	return nil
}

// NetNs retrieves the network namespace of the sandbox
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) NetNs() ns.NetNS {
	if s.netns == nil {
		return nil
	}

	return s.netns.ns
}

// NetNsPath returns the path to the network namespace
// If the sandbox uses the host namespace, "" is returned
func (s *Sandbox) NetNsPath() string {
	if s.netns == nil {
		return ""
	}

	return s.netns.symlink.Name()
}

// NetNsCreate creates a new network namespace for the sandbox
func (s *Sandbox) NetNsCreate() error {
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

		if err1 := s.netns.ns.Close(); err1 != nil {
			return err1
		}

		return err
	}

	return nil
}

// NetNsJoin attempts to join the sandbox to an existing network namespace
// This will fail if the sandbox is already part of a network namespace
func (s *Sandbox) NetNsJoin(nspath, name string) error {
	if s.netns != nil {
		return fmt.Errorf("sandbox already has a network namespace, cannot join another")
	}

	netNS, err := netNsGet(nspath, name)
	if err != nil {
		return err
	}

	s.netns = netNS

	return nil
}

// NetNsRemove removes the network namespace associated with the sandbox
func (s *Sandbox) NetNsRemove() error {
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
