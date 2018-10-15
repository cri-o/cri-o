package sandbox

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/kubernetes-sigs/cri-o/oci"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/network/hostport"
)

func isSymbolicLink(path string) (bool, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return false, err
	}

	return fi.Mode()&os.ModeSymlink == os.ModeSymlink, nil
}

// NetNsGet returns the NetNs associated with the given nspath and name
func NetNsGet(nspath, name string) (*NetNs, error) {
	if err := isNSorErr(nspath); err != nil {
		return nil, ErrClosedNetNS
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

	netNs, err := getNetNs(resolvedNsPath)
	if err != nil {
		return nil, err
	}

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

// HostNetNsPath returns the current network namespace for the host
func HostNetNsPath() (string, error) {
	return hostNetNsPath()
}

// Sandbox contains data surrounding kubernetes sandboxes on the server
type Sandbox struct {
	id        string
	namespace string
	// OCI pod name (eg "<namespace>-<name>-<attempt>")
	name string
	// Kubernetes pod name (eg, "<name>")
	kubeName       string
	logDir         string
	labels         fields.Set
	annotations    map[string]string
	infraContainer *oci.Container
	containers     oci.ContainerStorer
	processLabel   string
	mountLabel     string
	netns          *NetNs
	metadata       *pb.PodSandboxMetadata
	nsOpts         *pb.NamespaceOption
	shmPath        string
	cgroupParent   string
	privileged     bool
	trusted        bool
	runtimeHandler string
	resolvPath     string
	hostnamePath   string
	hostname       string
	portMappings   []*hostport.PortMapping
	created        bool
	stopped        bool
	// ipv4 or ipv6 cache
	ip                 string
	seccompProfilePath string
	createdAt          time.Time
	hostNetwork        bool
}

const (
	// DefaultShmSize is the default shm size
	DefaultShmSize = 64 * 1024 * 1024
	// NsRunDir is the default directory in which running network namespaces
	// are stored
	NsRunDir = "/var/run/netns"
	// PodInfraCommand is the default command when starting a pod infrastructure
	// container
	PodInfraCommand = "/pause"
)

var (
	// ErrIDEmpty is the erro returned when the id of the sandbox is empty
	ErrIDEmpty = errors.New("PodSandboxId should not be empty")
	// ErrClosedNetNS is the error returned when the network namespace of the
	// sandbox is closed
	ErrClosedNetNS = errors.New("PodSandbox networking namespace is closed")
)

// New creates and populates a new pod sandbox
// New sandboxes have no containers, no infra container, and no network namespaces associated with them
// An infra container must be attached before the sandbox is added to the state
func New(id, namespace, name, kubeName, logDir string, labels, annotations map[string]string, processLabel, mountLabel string, metadata *pb.PodSandboxMetadata, shmPath, cgroupParent string, privileged, trusted bool, runtimeHandler string, resolvPath, hostname string, portMappings []*hostport.PortMapping, hostNetwork bool) (*Sandbox, error) {
	sb := new(Sandbox)
	sb.id = id
	sb.namespace = namespace
	sb.name = name
	sb.kubeName = kubeName
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
	sb.trusted = trusted
	sb.runtimeHandler = runtimeHandler
	sb.resolvPath = resolvPath
	sb.hostname = hostname
	sb.portMappings = portMappings
	sb.createdAt = time.Now()
	sb.hostNetwork = hostNetwork

	return sb, nil
}

// SetSeccompProfilePath sets the seccomp profile path
func (s *Sandbox) SetSeccompProfilePath(pp string) {
	s.seccompProfilePath = pp
}

// SeccompProfilePath returns the seccomp profile path
func (s *Sandbox) SeccompProfilePath() string {
	return s.seccompProfilePath
}

// AddIP stores the ip in the sandbox
func (s *Sandbox) AddIP(ip string) {
	s.ip = ip
}

// SetNamespaceOptions sets whether the pod is running using host network
func (s *Sandbox) SetNamespaceOptions(nsOpts *pb.NamespaceOption) {
	s.nsOpts = nsOpts
}

// NamespaceOptions returns the namespace options for the sandbox
func (s *Sandbox) NamespaceOptions() *pb.NamespaceOption {
	return s.nsOpts
}

// IP returns the ip of the sandbox
func (s *Sandbox) IP() string {
	return s.ip
}

// ID returns the id of the sandbox
func (s *Sandbox) ID() string {
	return s.id
}

// Namespace returns the namespace for the sandbox
func (s *Sandbox) Namespace() string {
	return s.namespace
}

// Name returns the name of the sandbox
func (s *Sandbox) Name() string {
	return s.name
}

// KubeName returns the kubernetes name for the sandbox
func (s *Sandbox) KubeName() string {
	return s.kubeName
}

// LogDir returns the location of the logging directory for the sandbox
func (s *Sandbox) LogDir() string {
	return s.logDir
}

// Labels returns the labels associated with the sandbox
func (s *Sandbox) Labels() fields.Set {
	return s.labels
}

// Annotations returns a list of annotations for the sandbox
func (s *Sandbox) Annotations() map[string]string {
	return s.annotations
}

// InfraContainer returns the infrastructure container for the sandbox
func (s *Sandbox) InfraContainer() *oci.Container {
	return s.infraContainer
}

// Containers returns the ContainerStorer that contains information on all
// of the containers in the sandbox
func (s *Sandbox) Containers() oci.ContainerStorer {
	return s.containers
}

// ProcessLabel returns the process label for the sandbox
func (s *Sandbox) ProcessLabel() string {
	return s.processLabel
}

// MountLabel returns the mount label for the sandbox
func (s *Sandbox) MountLabel() string {
	return s.mountLabel
}

// Metadata returns a set of metadata about the sandbox
func (s *Sandbox) Metadata() *pb.PodSandboxMetadata {
	return s.metadata
}

// ShmPath returns the shm path of the sandbox
func (s *Sandbox) ShmPath() string {
	return s.shmPath
}

// CgroupParent returns the cgroup parent of the sandbox
func (s *Sandbox) CgroupParent() string {
	return s.cgroupParent
}

// Privileged returns whether or not the containers in the sandbox are
// privileged containers
func (s *Sandbox) Privileged() bool {
	return s.privileged
}

// Trusted returns whether or not the containers in the sandbox are trusted
func (s *Sandbox) Trusted() bool {
	return s.trusted
}

// RuntimeHandler returns the name of the runtime handler that should be
// picked from the list of runtimes. The name must match the key from the
// map of runtimes.
func (s *Sandbox) RuntimeHandler() string {
	return s.runtimeHandler
}

// HostNetwork returns whether the sandbox runs in the host network namespace
func (s *Sandbox) HostNetwork() bool {
	return s.hostNetwork
}

// ResolvPath returns the resolv path for the sandbox
func (s *Sandbox) ResolvPath() string {
	return s.resolvPath
}

// AddHostnamePath adds the hostname path to the sandbox
func (s *Sandbox) AddHostnamePath(hostname string) {
	s.hostnamePath = hostname
}

// HostnamePath retrieves the hostname path from a sandbox
func (s *Sandbox) HostnamePath() string {
	return s.hostnamePath
}

// Hostname returns the hsotname of the sandbox
func (s *Sandbox) Hostname() string {
	return s.hostname
}

// PortMappings returns a list of port mappings between the host and the sandbox
func (s *Sandbox) PortMappings() []*hostport.PortMapping {
	return s.portMappings
}

// AddContainer adds a container to the sandbox
func (s *Sandbox) AddContainer(c *oci.Container) {
	s.containers.Add(c.Name(), c)
}

// GetContainer retrieves a container from the sandbox
func (s *Sandbox) GetContainer(name string) *oci.Container {
	return s.containers.Get(name)
}

// RemoveContainer deletes a container from the sandbox
func (s *Sandbox) RemoveContainer(c *oci.Container) {
	s.containers.Delete(c.Name())
	intermediateMountPoint := c.IntermediateMountPoint()
	if intermediateMountPoint != "" {
		os.RemoveAll(intermediateMountPoint)
	}
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

// RemoveInfraContainer removes the infrastructure container of a sandbox
func (s *Sandbox) RemoveInfraContainer() {
	s.infraContainer = nil
}

// NetNs retrieves the network namespace of the sandbox
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) NetNs() *NetNs {
	return s.netns
}

// NetNsPath returns the path to the network namespace of the sandbox.
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) NetNsPath() string {
	if s.netns == nil || s.netns.symlink == nil {
		if s.infraContainer != nil {
			return fmt.Sprintf("/proc/%v/ns/net", s.infraContainer.State().Pid)
		}
		return ""
	}

	return s.netns.symlink.Name()
}

// UserNsPath returns the path to the user namespace of the sandbox.
// If the sandbox uses the host namespace, nil is returned
func (s *Sandbox) UserNsPath() string {
	if s.infraContainer != nil {
		return fmt.Sprintf("/proc/%v/ns/user", s.infraContainer.State().Pid)
	}
	return ""
}

// NetNsCreate creates a new network namespace for the sandbox
func (s *Sandbox) NetNsCreate() error {
	if s.netns != nil {
		return fmt.Errorf("net NS already created")
	}

	netNS, err := newNetNs()
	if err != nil {
		return err
	}
	s.netns = netNS

	if err := s.netns.symlinkCreate(s.name); err != nil {
		logrus.Warnf("Could not create nentns symlink %v", err)

		if err1 := s.netns.Close(); err1 != nil {
			return err1
		}

		return err
	}

	return nil
}

// SetStopped sets the sandbox state to stopped.
// This should be set after a stop operation succeeds
// so that subsequent stops can return fast.
func (s *Sandbox) SetStopped() {
	s.stopped = true
}

// Stopped returns whether the sandbox state has been
// set to stopped.
func (s *Sandbox) Stopped() bool {
	return s.stopped
}

// NetNsJoin attempts to join the sandbox to an existing network namespace
// This will fail if the sandbox is already part of a network namespace
func (s *Sandbox) NetNsJoin(nspath, name string) error {
	if s.netns != nil {
		return fmt.Errorf("sandbox already has a network namespace, cannot join another")
	}

	netNS, err := NetNsGet(nspath, name)
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

	return s.netns.Remove()
}

// SetCreated sets the created status of sandbox to true
func (s *Sandbox) SetCreated() {
	s.created = true
}

// Created returns the created status of sandbox
func (s *Sandbox) Created() bool {
	return s.created
}
