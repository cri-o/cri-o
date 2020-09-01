package oci

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/containers/libpod/pkg/cgroups"
	"github.com/containers/storage/pkg/idtools"
	json "github.com/json-iterator/go"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/fields"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/types"
)

const defaultStopSignalInt = 15

var (
	defaultStopSignal   = strconv.Itoa(defaultStopSignalInt)
	ErrContainerStopped = errors.New("container is already stopped")
	ErrNotFound         = errors.New("container process not found")
)

// Container represents a runtime container.
type Container struct {
	volumes        []ContainerVolume
	id             string
	name           string
	logPath        string
	image          string
	sandbox        string
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
	ExitCode  *int32    `json:"exitCode,omitempty"`
	OOMKilled bool      `json:"oomKilled,omitempty"`
	Error     string    `json:"error,omitempty"`
	InitPid   int       `json:"initPid,omitempty"`
	// The unix start time of the container's init PID.
	// This is used to track whether the PID we have stored
	// is the same as the corresponding PID on the host.
	InitStartTime int `json:"initStartTime,omitempty"`
}

// NewContainer creates a container object.
func NewContainer(id, name, bundlePath, logPath string, labels, crioAnnotations, annotations map[string]string, image, imageName, imageRef string, metadata *pb.ContainerMetadata, sandbox string, terminal, stdin, stdinOnce bool, runtimeHandler, dir string, created time.Time, stopSignal string) (*Container, error) {
	state := &ContainerState{}
	state.Created = created
	c := &Container{
		id:              id,
		name:            name,
		bundlePath:      bundlePath,
		logPath:         logPath,
		labels:          labels,
		sandbox:         sandbox,
		terminal:        terminal,
		stdin:           stdin,
		stdinOnce:       stdinOnce,
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
	signal := unix.SignalNum(strings.ToUpper(c.stopSignal))
	if signal == 0 {
		return defaultStopSignal
	}
	// return the stop signal in the form of its int converted to a string
	// i.e stop signal 34 is returned as "34" to avoid back and forth conversion
	return strconv.Itoa(int(signal))
}

// StopSignal returns the container's own stop signal configured from
// the image configuration or the default one.
func (c *Container) StopSignal() syscall.Signal {
	if c.stopSignal == "" {
		return defaultStopSignalInt
	}

	signal := unix.SignalNum(strings.ToUpper(c.stopSignal))
	if signal == 0 {
		return defaultStopSignalInt
	}
	return signal
}

// FromDisk restores container's state from disk
// Calls to FromDisk should always be preceded by call to Runtime.UpdateContainerStatus.
// This is because FromDisk() initializes the InitStartTime for the saved container state
// when CRI-O is being upgraded to a version that supports tracking PID,
// but does no verification the container is actually still running. If we assume the container
// is still running, we could incorrectly think a process with the same PID running on the host
// is our container. A call to `$runtime state` will protect us against this.
func (c *Container) FromDisk() error {
	jsonSource, err := os.Open(c.StatePath())
	if err != nil {
		return err
	}
	defer jsonSource.Close()

	dec := json.NewDecoder(jsonSource)
	tmpState := &ContainerState{}
	if err := dec.Decode(tmpState); err != nil {
		return err
	}

	// this is to handle the situation in which we're upgrading
	// versions of cri-o, and we didn't used to have this information in the state
	if tmpState.InitPid == 0 && tmpState.InitStartTime == 0 && tmpState.Pid != 0 {
		if err := tmpState.SetInitPid(tmpState.Pid); err != nil {
			return err
		}
		logrus.Infof("PID information for container %s updated to %d %d", c.id, tmpState.InitPid, tmpState.InitStartTime)
	}
	c.state = tmpState
	return nil
}

// SetInitPid initializes the InitPid and InitStartTime for the container state
// given a PID.
// These values should be set once, and not changed again.
func (cstate *ContainerState) SetInitPid(pid int) error {
	if cstate.InitPid != 0 || cstate.InitStartTime != 0 {
		return errors.Errorf("pid and start time already initialized: %d %d", cstate.InitPid, cstate.InitStartTime)
	}
	cstate.InitPid = pid
	startTime, err := getPidStartTime(pid)
	if err != nil {
		return err
	}
	cstate.InitStartTime = startTime
	return nil
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
	if c == nil {
		return ""
	}
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

func (c *Container) exitFilePath() string {
	return filepath.Join(c.dir, "exit")
}

// IsAlive is a function that checks if a container's init PID exists.
// It is used to check a container state when we don't want a `$runtime state` call
func (c *Container) IsAlive() error {
	_, err := c.pid()
	return errors.Wrapf(err, "checking if PID of %s is running failed", c.id)
}

// Pid returns the container's init PID.
// It will fail if the saved PID no longer belongs to the container.
func (c *Container) Pid() (int, error) {
	c.opLock.Lock()
	defer c.opLock.Unlock()
	return c.pid()
}

// pid returns the container's init PID.
// It checks that we have an InitPid defined in the state, that PID can be found
// and it is the same process that was originally started by the runtime.
func (c *Container) pid() (int, error) {
	if c.state == nil {
		return 0, errors.New("state not initialized")
	}
	if c.state.InitPid <= 0 {
		return 0, errors.New("PID not initialized")
	}

	// container has stopped (as pid is initialized but the runc state has overwritten it)
	if c.state.Pid == 0 {
		return 0, ErrNotFound
	}

	if err := c.verifyPid(); err != nil {
		return 0, err
	}
	return c.state.InitPid, nil
}

// verifyPid checks that the start time for the process on the node is the same
// as the start time we saved after creating the container.
// This is the simplest way to verify we are operating on the container
// process, and haven't run into PID wrap.
func (c *Container) verifyPid() error {
	startTime, err := getPidStartTime(c.state.InitPid)
	if err != nil {
		return err
	}

	if startTime != c.state.InitStartTime {
		return errors.New("PID running but not the original container. PID wrap may have occurred")
	}
	return nil
}

// getPidStartTime reads the kernel's /proc entry for stime for PID.
func getPidStartTime(pid int) (int, error) {
	var st unix.Stat_t
	if err := unix.Stat(fmt.Sprintf("/proc/%d", pid), &st); err != nil {
		return 0, errors.Wrapf(ErrNotFound, err.Error())
	}

	return int(st.Ctim.Sec), nil
}

// ShouldBeStopped checks whether the container state is in a place
// where attempting to stop it makes sense
// a container is not stoppable if it's paused or stopped
// if it's paused, that's an error, and is reported as such
func (c *Container) ShouldBeStopped() error {
	switch c.state.Status {
	case ContainerStateStopped: // no-op
		return ErrContainerStopped
	case ContainerStatePaused:
		return errors.New("cannot stop paused container")
	}
	return nil
}
