package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/fields"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/nsmgr"
	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/memorystore"
	"github.com/cri-o/cri-o/internal/oci"
)

// DevShmPath is the default system wide shared memory path.
const DevShmPath = "/dev/shm"

var (
	sbStoppedFilename        = "stopped"
	sbNetworkStoppedFilename = "network-stopped"
)

// Sandbox contains data surrounding kubernetes sandboxes on the server.
type Sandbox struct {
	criSandbox   *types.PodSandbox
	portMappings []*hostport.PortMapping
	namespace    string
	// OCI pod name (eg "<namespace>-<name>-<attempt>")
	name string
	// Kubernetes pod name (eg, "<name>")
	kubeName       string
	logDir         string
	containers     memorystore.Storer[*oci.Container]
	createdAt      time.Time
	processLabel   string
	mountLabel     string
	netns          nsmgr.Namespace
	ipcns          nsmgr.Namespace
	utsns          nsmgr.Namespace
	userns         nsmgr.Namespace
	shmPath        string
	cgroupParent   string
	runtimeHandler string
	resolvPath     string
	hostnamePath   string
	hostname       string
	// ipv4 or ipv6 cache
	ips                []string
	seccompProfilePath string
	infraContainer     *oci.Container
	nsOpts             *types.NamespaceOption
	dnsConfig          *types.DNSConfig
	stopMutex          sync.RWMutex
	// stateMutex protects the use of created, stopped and networkStopped bools
	// which are all fields that can change at runtime
	stateMutex        sync.RWMutex
	created           bool
	stopped           bool
	networkStopped    bool
	privileged        bool
	hostNetwork       bool
	usernsMode        string
	containerEnvPath  string
	podLinuxOverhead  *types.LinuxContainerResources
	podLinuxResources *types.LinuxContainerResources
}

// DefaultShmSize is the default shm size.
const DefaultShmSize = 64 * 1024 * 1024

// ErrIDEmpty is the error returned when the id of the sandbox is empty.
var ErrIDEmpty = errors.New("PodSandboxId should not be empty")

func (s *Sandbox) CRISandbox() *types.PodSandbox {
	// If a protobuf message gets mutated mid-request, then the proto library panics.
	// We would like to avoid deep copies when possible to avoid excessive garbage
	// collection, but need to if the sandbox changes state.
	newState := s.State()
	if newState != s.criSandbox.GetState() {
		cpy := proto.CloneOf(s.criSandbox)
		cpy.State = newState
		s.criSandbox = cpy
	}

	return s.criSandbox
}

func (s *Sandbox) CreatedAt() time.Time {
	return s.createdAt
}

// SetSeccompProfilePath sets the seccomp profile path.
func (s *Sandbox) SetSeccompProfilePath(pp string) {
	s.seccompProfilePath = pp
}

// SeccompProfilePath returns the seccomp profile path.
func (s *Sandbox) SeccompProfilePath() string {
	return s.seccompProfilePath
}

// AddIPs stores the ip in the sandbox.
func (s *Sandbox) AddIPs(ips []string) {
	s.ips = ips
}

// SetNamespaceOptions sets whether the pod is running using host network.
func (s *Sandbox) SetNamespaceOptions(nsOpts *types.NamespaceOption) {
	s.nsOpts = nsOpts
}

// NamespaceOptions returns the namespace options for the sandbox.
func (s *Sandbox) NamespaceOptions() *types.NamespaceOption {
	return s.nsOpts
}

// SetDNSConfig sets the DNSConfig.
func (s *Sandbox) SetDNSConfig(dnsConfig *types.DNSConfig) {
	s.dnsConfig = dnsConfig
}

// DNSConfig returns the dnsConfig for the sandbox.
func (s *Sandbox) DNSConfig() *types.DNSConfig {
	return s.dnsConfig
}

// StopMutex returns the mutex to use when stopping the sandbox.
func (s *Sandbox) StopMutex() *sync.RWMutex {
	return &s.stopMutex
}

// IPs returns the ip of the sandbox.
func (s *Sandbox) IPs() []string {
	return s.ips
}

// GetIPs returns the ip of the sandbox.
// Wrap the IPs() method to match the NRI interface.
func (s *Sandbox) GetIPs() []string {
	return s.IPs()
}

// ID returns the id of the sandbox.
func (s *Sandbox) ID() string {
	return s.criSandbox.GetId()
}

// UsernsMode returns the mode for setting the user namespace, if any.
func (s *Sandbox) UsernsMode() string {
	return s.usernsMode
}

// Namespace returns the namespace for the sandbox.
func (s *Sandbox) Namespace() string {
	return s.namespace
}

// Name returns the name of the sandbox.
func (s *Sandbox) Name() string {
	return s.name
}

// KubeName returns the kubernetes name for the sandbox.
func (s *Sandbox) KubeName() string {
	return s.kubeName
}

// LogDir returns the location of the logging directory for the sandbox.
func (s *Sandbox) LogDir() string {
	return s.logDir
}

// Labels returns the labels associated with the sandbox.
func (s *Sandbox) Labels() fields.Set {
	return s.criSandbox.GetLabels()
}

// Annotations returns a list of annotations for the sandbox.
func (s *Sandbox) Annotations() map[string]string {
	return s.criSandbox.GetAnnotations()
}

// Containers returns the ContainerStorer that contains information on all
// of the containers in the sandbox.
func (s *Sandbox) Containers() memorystore.Storer[*oci.Container] {
	return s.containers
}

// ProcessLabel returns the process label for the sandbox.
func (s *Sandbox) ProcessLabel() string {
	return s.processLabel
}

// MountLabel returns the mount label for the sandbox.
func (s *Sandbox) MountLabel() string {
	return s.mountLabel
}

// Metadata returns a set of metadata about the sandbox.
func (s *Sandbox) Metadata() *types.PodSandboxMetadata {
	return s.criSandbox.GetMetadata()
}

// ShmPath returns the shm path of the sandbox.
func (s *Sandbox) ShmPath() string {
	return s.shmPath
}

// CgroupParent returns the cgroup parent of the sandbox.
func (s *Sandbox) CgroupParent() string {
	return s.cgroupParent
}

// Privileged returns whether or not the containers in the sandbox are
// privileged containers.
func (s *Sandbox) Privileged() bool {
	return s.privileged
}

// RuntimeHandler returns the name of the runtime handler that should be
// picked from the list of runtimes. The name must match the key from the
// map of runtimes.
func (s *Sandbox) RuntimeHandler() string {
	return s.runtimeHandler
}

// HostNetwork returns whether the sandbox runs in the host network namespace.
func (s *Sandbox) HostNetwork() bool {
	return s.hostNetwork
}

// ResolvPath returns the resolv path for the sandbox.
func (s *Sandbox) ResolvPath() string {
	return s.resolvPath
}

// AddHostnamePath adds the hostname path to the sandbox.
func (s *Sandbox) AddHostnamePath(hostname string) {
	s.hostnamePath = hostname
}

// HostnamePath retrieves the hostname path from a sandbox.
func (s *Sandbox) HostnamePath() string {
	return s.hostnamePath
}

// ContainerEnvPath retrieves the .containerenv path from a sandbox.
func (s *Sandbox) ContainerEnvPath() string {
	return s.containerEnvPath
}

// Hostname returns the hostname of the sandbox.
func (s *Sandbox) Hostname() string {
	return s.hostname
}

// PortMappings returns a list of port mappings between the host and the sandbox.
func (s *Sandbox) PortMappings() []*hostport.PortMapping {
	return s.portMappings
}

// PodLinuxOverhead returns the overheads associated with this sandbox.
func (s *Sandbox) PodLinuxOverhead() *types.LinuxContainerResources {
	return s.podLinuxOverhead
}

// PodLinuxResources returns the sum of container resources for this sandbox.
func (s *Sandbox) PodLinuxResources() *types.LinuxContainerResources {
	return s.podLinuxResources
}

// AddContainer adds a container to the sandbox.
func (s *Sandbox) AddContainer(ctx context.Context, c *oci.Container) {
	_, span := log.StartSpan(ctx)
	defer span.End()

	s.containers.Add(c.Name(), c)
}

// GetContainer retrieves a container from the sandbox.
func (s *Sandbox) GetContainer(ctx context.Context, name string) *oci.Container {
	_, span := log.StartSpan(ctx)
	defer span.End()

	return s.containers.Get(name)
}

// RemoveContainer deletes a container from the sandbox.
func (s *Sandbox) RemoveContainer(ctx context.Context, c *oci.Container) {
	_, span := log.StartSpan(ctx)
	defer span.End()

	s.containers.Delete(c.Name())
}

// SetInfraContainer sets the infrastructure container of a sandbox
// Attempts to set the infrastructure container after one is already present will throw an error.
func (s *Sandbox) SetInfraContainer(infraCtr *oci.Container) error {
	if s.infraContainer != nil {
		return errors.New("sandbox already has an infra container")
	} else if infraCtr == nil {
		return errors.New("must provide non-nil infra container")
	}

	s.infraContainer = infraCtr

	return nil
}

// InfraContainer returns the infrastructure container for the sandbox.
func (s *Sandbox) InfraContainer() *oci.Container {
	return s.infraContainer
}

// RemoveInfraContainer removes the infrastructure container of a sandbox.
func (s *Sandbox) RemoveInfraContainer() {
	s.infraContainer = nil
}

// SetStopped sets the sandbox state to stopped.
// This should be set after a stop operation succeeds
// so that subsequent stops can return fast.
// if createFile is true, it also creates a "stopped" file in the infra container's persistent dir
// this is used to track the sandbox is stopped over reboots.
func (s *Sandbox) SetStopped(ctx context.Context, createFile bool) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()

	if s.stopped {
		return
	}

	s.stopped = true
	if createFile {
		if err := s.createFileInInfraDir(ctx, sbStoppedFilename); err != nil {
			log.Errorf(ctx, "Failed to create stopped file in container state. Restore may fail: %v", err)
		}
	}
}

// Stopped returns whether the sandbox state has been
// set to stopped.
func (s *Sandbox) Stopped() bool {
	s.stateMutex.RLock()
	defer s.stateMutex.RUnlock()

	return s.stopped
}

// SetCreated sets the created status of sandbox to true.
func (s *Sandbox) SetCreated() {
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()

	s.created = true
}

// NetworkStopped returns whether the network has been stopped.
func (s *Sandbox) NetworkStopped() bool {
	s.stateMutex.RLock()
	defer s.stateMutex.RUnlock()

	return s.networkStopped
}

// SetNetworkStopped sets the sandbox network state as stopped
// This should be set after a network stop operation succeeds,
// so we don't double stop the network
// if createFile is true, it creates a "network-stopped" file
// in the infra container's persistent dir
// this is used to track the network is stopped over reboots
// returns an error if an error occurred when creating the network-stopped file.
func (s *Sandbox) SetNetworkStopped(ctx context.Context, createFile bool) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()

	if s.networkStopped {
		return nil
	}

	s.networkStopped = true
	if createFile {
		if err := s.createFileInInfraDir(ctx, sbNetworkStoppedFilename); err != nil {
			return fmt.Errorf("failed to create state file in container directory. Restores may fail: %w", err)
		}
	}

	return nil
}

// SetContainerEnvFile sets the container environment file.
func (s *Sandbox) SetContainerEnvFile(ctx context.Context) error {
	_, span := log.StartSpan(ctx)
	defer span.End()

	if s.containerEnvPath != "" {
		return nil
	}

	infra := s.InfraContainer()
	filePath := filepath.Join(infra.Dir(), ".containerenv")

	f, err := os.Create(filePath)
	if err == nil {
		f.Close()
	}

	s.containerEnvPath = filePath

	return nil
}

// This function assumes the state lock has been taken for this sandbox.
func (s *Sandbox) createFileInInfraDir(ctx context.Context, filename string) error {
	// If the sandbox is not yet created,
	// this function is being called when
	// cleaning up a failed sandbox creation.
	// We don't need to create the file, as there will be no
	// sandbox to restore
	_, span := log.StartSpan(ctx)
	defer span.End()

	if !s.created {
		return nil
	}

	infra := s.InfraContainer()
	// If the infra directory has been cleaned up already, we should not fail to
	// create this file.
	if _, err := os.Stat(infra.Dir()); os.IsNotExist(err) {
		return nil
	}

	f, err := os.Create(filepath.Join(infra.Dir(), filename))
	if err == nil {
		f.Close()
	}

	return err
}

func (s *Sandbox) RestoreStopped() {
	s.stateMutex.Lock()
	defer s.stateMutex.Unlock()

	if s.fileExistsInInfraDir(sbStoppedFilename) {
		s.stopped = true
	}

	if s.fileExistsInInfraDir(sbNetworkStoppedFilename) {
		s.networkStopped = true
	}
}

func (s *Sandbox) fileExistsInInfraDir(filename string) bool {
	infra := s.InfraContainer()

	infraFilePath := filepath.Join(infra.Dir(), filename)
	if _, err := os.Stat(infraFilePath); err != nil {
		if !os.IsNotExist(err) {
			logrus.Warnf("Error checking if %s exists: %v", infraFilePath, err)
		}

		return false
	}

	return true
}

// Created returns the created status of sandbox.
func (s *Sandbox) Created() bool {
	s.stateMutex.RLock()
	defer s.stateMutex.RUnlock()

	return s.created
}

func (s *Sandbox) State() types.PodSandboxState {
	if s.Ready() {
		return types.PodSandboxState_SANDBOX_READY
	}

	return types.PodSandboxState_SANDBOX_NOTREADY
}

// Ready returns whether the sandbox should be marked as ready to the kubelet
// if there is no infra container, it is always considered ready.
// `takeLock` should be set if we need to take the lock to get the infra container's state.
// If there is no infra container, it is never considered ready.
// If the infra container is spoofed, the pod is considered ready when it has been created, but not stopped.
func (s *Sandbox) Ready() bool {
	s.stateMutex.RLock()
	defer s.stateMutex.RUnlock()

	return s.created && !s.stopped
}
