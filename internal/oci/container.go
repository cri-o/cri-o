package oci

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/containers/libpod/pkg/cgroups"
	"github.com/containers/storage/pkg/idtools"
	"github.com/docker/docker/pkg/signal"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/fields"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/types"
)

const (
	defaultStopSignal    = "TERM"
	defaultStopSignalInt = 15
)

// Container represents a runtime container.
type Container struct {
	volumes        []ContainerVolume
	id             string
	name           string
	logPath        string
	image          string
	sandbox        string
	netns          string
	runtimeHandler string
	// this is the /var/run/storage/... directory, erased on reboot
	bundlePath string
	// this is the /var/lib/storage/... directory
	dir                string
	stopSignal         string
	imageName          string
	imageRef           string
	mountPoint         string
	seccompProfilePath string
	conmonCgroupfsPath string
	labels             fields.Set
	annotations        fields.Set
	crioAnnotations    fields.Set
	state              *ContainerState
	metadata           *pb.ContainerMetadata
	opLock             sync.RWMutex
	spec               *specs.Spec
	idMappings         *idtools.IDMappings
	terminal           bool
	stdin              bool
	stdinOnce          bool
	privileged         bool
	created            bool
}

// ContainerVolume is a bind mount for the container.
type ContainerVolume struct {
	ContainerPath string `json:"container_path"`
	HostPath      string `json:"host_path"`
	Readonly      bool   `json:"readonly"`
}

// ContainerState represents the status of a container.
type ContainerState struct {
	specs.State
	Created   time.Time `json:"created"`
	Started   time.Time `json:"started,omitempty"`
	Finished  time.Time `json:"finished,omitempty"`
	ExitCode  int32     `json:"exitCode,omitempty"`
	OOMKilled bool      `json:"oomKilled,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// NewContainer creates a container object.
func NewContainer(id, name, bundlePath, logPath, netns string, labels, crioAnnotations, annotations map[string]string, image, imageName, imageRef string, metadata *pb.ContainerMetadata, sandbox string, terminal, stdin, stdinOnce, privileged bool, runtimeHandler, dir string, created time.Time, stopSignal string) (*Container, error) {
	state := &ContainerState{}
	state.Created = created
	c := &Container{
		id:              id,
		name:            name,
		bundlePath:      bundlePath,
		logPath:         logPath,
		labels:          labels,
		sandbox:         sandbox,
		netns:           netns,
		terminal:        terminal,
		stdin:           stdin,
		stdinOnce:       stdinOnce,
		privileged:      privileged,
		runtimeHandler:  runtimeHandler,
		metadata:        metadata,
		annotations:     annotations,
		crioAnnotations: crioAnnotations,
		image:           image,
		imageName:       imageName,
		imageRef:        imageRef,
		dir:             dir,
		state:           state,
		stopSignal:      stopSignal,
	}
	return c, nil
}

// SetSpec loads the OCI spec in the container struct
func (c *Container) SetSpec(s *specs.Spec) {
	c.spec = s
}

// Spec returns a copy of the spec for the container
func (c *Container) Spec() specs.Spec {
	return *c.spec
}

// ConmonCgroupfsPath returns the path to conmon's cgroup. This is only set when
// cgroupfs is used as a cgroup manager.
func (c *Container) ConmonCgroupfsPath() string {
	return c.conmonCgroupfsPath
}

// GetStopSignal returns the container's own stop signal configured from the
// image configuration or the default one.
func (c *Container) GetStopSignal() string {
	if c.stopSignal == "" {
		return defaultStopSignal
	}
	cleanSignal := strings.TrimPrefix(strings.ToUpper(c.stopSignal), "SIG")
	_, ok := signal.SignalMap[cleanSignal]
	if !ok {
		return defaultStopSignal
	}
	return cleanSignal
}

// StopSignal returns the container's own stop signal configured from
// the image configuration or the default one.
func (c *Container) StopSignal() syscall.Signal {
	if c.stopSignal == "" {
		return defaultStopSignalInt
	}
	cleanSignal := strings.TrimPrefix(strings.ToUpper(c.stopSignal), "SIG")
	sig, ok := signal.SignalMap[cleanSignal]
	if !ok {
		return defaultStopSignalInt
	}
	return sig
}

// FromDisk restores container's state from disk
func (c *Container) FromDisk() error {
	jsonSource, err := os.Open(c.StatePath())
	if err != nil {
		return err
	}
	defer jsonSource.Close()

	dec := json.NewDecoder(jsonSource)
	return dec.Decode(c.state)
}

// StatePath returns the containers state.json path
func (c *Container) StatePath() string {
	return filepath.Join(c.dir, "state.json")
}

// CreatedAt returns the container creation time
func (c *Container) CreatedAt() time.Time {
	return c.state.Created
}

// Name returns the name of the container.
func (c *Container) Name() string {
	return c.name
}

// ID returns the id of the container.
func (c *Container) ID() string {
	return c.id
}

// CleanupConmonCgroup cleans up conmon's group when using cgroupfs.
func (c *Container) CleanupConmonCgroup() {
	path := c.ConmonCgroupfsPath()
	if path == "" {
		return
	}
	cg, err := cgroups.Load(path)
	if err != nil {
		logrus.Infof("error loading conmon cgroup of container %s: %v", c.ID(), err)
		return
	}
	if err := cg.Delete(); err != nil {
		logrus.Infof("error deleting conmon cgroup of container %s: %v", c.ID(), err)
	}
}

// SetSeccompProfilePath sets the seccomp profile path
func (c *Container) SetSeccompProfilePath(pp string) {
	c.seccompProfilePath = pp
}

// SeccompProfilePath returns the seccomp profile path
func (c *Container) SeccompProfilePath() string {
	return c.seccompProfilePath
}

// BundlePath returns the bundlePath of the container.
func (c *Container) BundlePath() string {
	return c.bundlePath
}

// LogPath returns the log path of the container.
func (c *Container) LogPath() string {
	return c.logPath
}

// Labels returns the labels of the container.
func (c *Container) Labels() map[string]string {
	return c.labels
}

// Annotations returns the annotations of the container.
func (c *Container) Annotations() map[string]string {
	return c.annotations
}

// CrioAnnotations returns the crio annotations of the container.
func (c *Container) CrioAnnotations() map[string]string {
	return c.crioAnnotations
}

// Image returns the image of the container.
func (c *Container) Image() string {
	return c.image
}

// ImageName returns the image name of the container.
func (c *Container) ImageName() string {
	return c.imageName
}

// ImageRef returns the image ref of the container.
func (c *Container) ImageRef() string {
	return c.imageRef
}

// Sandbox returns the sandbox name of the container.
func (c *Container) Sandbox() string {
	return c.sandbox
}

// Dir returns the dir of the container
func (c *Container) Dir() string {
	return c.dir
}

// NetNsPath returns the path to the network namespace of the container.
func (c *Container) NetNsPath() (string, error) {
	if c.state == nil {
		return "", fmt.Errorf("container state is not populated")
	}

	if c.netns == "" {
		return fmt.Sprintf("/proc/%d/ns/net", c.state.Pid), nil
	}

	return c.netns, nil
}

// Metadata returns the metadata of the container.
func (c *Container) Metadata() *pb.ContainerMetadata {
	return c.metadata
}

// State returns the state of the running container
func (c *Container) State() *ContainerState {
	c.opLock.RLock()
	defer c.opLock.RUnlock()
	return c.state
}

// StateNoLock returns the state of a container without using a lock.
func (c *Container) StateNoLock() *ContainerState {
	return c.state
}

// AddVolume adds a volume to list of container volumes.
func (c *Container) AddVolume(v ContainerVolume) {
	c.volumes = append(c.volumes, v)
}

// Volumes returns the list of container volumes.
func (c *Container) Volumes() []ContainerVolume {
	return c.volumes
}

// SetMountPoint sets the container mount point
func (c *Container) SetMountPoint(mp string) {
	c.mountPoint = mp
}

// MountPoint returns the container mount point
func (c *Container) MountPoint() string {
	return c.mountPoint
}

// SetIDMappings sets the ID/GID mappings used for the container
func (c *Container) SetIDMappings(mappings *idtools.IDMappings) {
	c.idMappings = mappings
}

// IDMappings returns the ID/GID mappings used for the container
func (c *Container) IDMappings() *idtools.IDMappings {
	return c.idMappings
}

// SetCreated sets the created flag to true once container is created
func (c *Container) SetCreated() {
	c.created = true
}

// Created returns whether the container was created successfully
func (c *Container) Created() bool {
	return c.created
}

// SetStartFailed sets the container state appropriately after a start failure
func (c *Container) SetStartFailed(err error) {
	c.opLock.Lock()
	defer c.opLock.Unlock()
	// adjust finished and started times
	c.state.Finished, c.state.Started = c.state.Created, c.state.Created
	if err != nil {
		c.state.Error = err.Error()
	}
}

// Description returns a description for the container
func (c *Container) Description() string {
	return fmt.Sprintf("%s/%s/%s", c.Labels()[types.KubernetesPodNamespaceLabel], c.Labels()[types.KubernetesPodNameLabel], c.Labels()[types.KubernetesContainerNameLabel])
}

// StdinOnce returns whether stdin once is set for the container.
func (c *Container) StdinOnce() bool {
	return c.stdinOnce
}
