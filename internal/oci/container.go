package oci

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/containers/common/pkg/cgroups"
	"github.com/containers/common/pkg/signal"
	"github.com/containers/storage/pkg/idtools"
	"github.com/cri-o/cri-o/internal/config/nsmgr"
	ann "github.com/cri-o/cri-o/pkg/annotations"
	json "github.com/json-iterator/go"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/fields"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubeletTypes "k8s.io/kubernetes/pkg/kubelet/types"
)

const (
	defaultStopSignalInt = 15

	procStatFile = "/proc/%d/stat"

	// Fields from the /proc/<PID>/stat file. see:
	//   https://man7.org/linux/man-pages/man5/proc.5.html
	//
	// Field no. 3, the process state, such as "R", "S", "D", etc.
	// Field no. 22, the process start time, using clock ticks since the system boot.
	//
	// The index values are shifted three fields to the left
	// with the process name field skipped over during parsing.
	stateFieldIndex     = 0
	startTimeFieldIndex = 19
)

var (
	ErrContainerStopped = errors.New("container is already stopped")
	ErrNotFound         = errors.New("container process not found")
	ErrNotInitialized   = errors.New("container PID not initialized")
)

// Container represents a runtime container.
type Container struct {
	criContainer   *types.Container
	volumes        []ContainerVolume
	name           string
	logPath        string
	runtimeHandler string
	// this is the /var/run/storage/... directory, erased on reboot
	bundlePath string
	// this is the /var/lib/storage/... directory
	dir                string
	stopSignal         string
	imageName          string
	mountPoint         string
	seccompProfilePath string
	conmonCgroupfsPath string
	crioAnnotations    fields.Set
	state              *ContainerState
	opLock             sync.RWMutex
	spec               *specs.Spec
	idMappings         *idtools.IDMappings
	terminal           bool
	stdin              bool
	stdinOnce          bool
	created            bool
	spoofed            bool
	stopping           bool
	stopLock           sync.Mutex
	stopTimeoutChan    chan int64
	stopWatchers       []chan struct{}
	pidns              nsmgr.Namespace
	restore            bool
	restoreArchive     string
	execPIDs           map[int]bool
}

func (c *Container) CRIAttributes() *types.ContainerAttributes {
	return &types.ContainerAttributes{
		Id:          c.ID(),
		Metadata:    c.Metadata(),
		Labels:      c.Labels(),
		Annotations: c.Annotations(),
	}
}

// ContainerVolume is a bind mount for the container.
type ContainerVolume struct {
	ContainerPath  string                 `json:"container_path"`
	HostPath       string                 `json:"host_path"`
	Readonly       bool                   `json:"readonly"`
	Propagation    types.MountPropagation `json:"propagation"`
	SelinuxRelabel bool                   `json:"selinux_relabel"`
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
	InitStartTime string `json:"initStartTime,omitempty"`
}

// NewContainer creates a container object.
func NewContainer(id, name, bundlePath, logPath string, labels, crioAnnotations, annotations map[string]string, image, imageName, imageRef string, md *types.ContainerMetadata, sandbox string, terminal, stdin, stdinOnce bool, runtimeHandler, dir string, created time.Time, stopSignal string) (*Container, error) {
	state := &ContainerState{}
	state.Created = created
	c := &Container{
		criContainer: &types.Container{
			Id:           id,
			PodSandboxId: sandbox,
			CreatedAt:    created.UnixNano(),
			Labels:       labels,
			Metadata:     md,
			Annotations:  annotations,
			Image: &types.ImageSpec{
				Image: image,
			},
			ImageRef: imageRef,
		},
		name:            name,
		bundlePath:      bundlePath,
		logPath:         logPath,
		terminal:        terminal,
		stdin:           stdin,
		stdinOnce:       stdinOnce,
		runtimeHandler:  runtimeHandler,
		crioAnnotations: crioAnnotations,
		imageName:       imageName,
		dir:             dir,
		state:           state,
		stopSignal:      stopSignal,
		stopTimeoutChan: make(chan int64, 10),
		stopWatchers:    []chan struct{}{},
		execPIDs:        map[int]bool{},
	}
	return c, nil
}

func NewSpoofedContainer(id, name string, labels map[string]string, sandbox string, created time.Time, dir string) *Container {
	state := &ContainerState{}
	state.Created = created
	state.Started = created
	c := &Container{
		criContainer: &types.Container{
			Id:           id,
			CreatedAt:    created.UnixNano(),
			Labels:       labels,
			PodSandboxId: sandbox,
			Metadata:     &types.ContainerMetadata{},
			Annotations: map[string]string{
				ann.SpoofedContainer: "true",
			},
			Image: &types.ImageSpec{},
		},
		name:    name,
		spoofed: true,
		state:   state,
		dir:     dir,
	}
	return c
}

func (c *Container) CRIContainer() *types.Container {
	// If a protobuf message gets mutated mid-request, then the proto library panics.
	// We would like to avoid deep copies when possible to avoid excessive garbage
	// collection, but need to if the container changes state.
	newState := types.ContainerState_CONTAINER_UNKNOWN
	switch c.StateNoLock().Status {
	case ContainerStateCreated:
		newState = types.ContainerState_CONTAINER_CREATED
	case ContainerStateRunning, ContainerStatePaused:
		newState = types.ContainerState_CONTAINER_RUNNING
	case ContainerStateStopped:
		newState = types.ContainerState_CONTAINER_EXITED
	}
	if newState != c.criContainer.State {
		cpy := *c.criContainer
		cpy.State = newState
		c.criContainer = &cpy
	}
	return c.criContainer
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
	// return the stop signal in the form of its int converted to a string
	// i.e stop signal 34 is returned as "34" to avoid back and forth conversion
	return strconv.Itoa(int(c.StopSignal()))
}

// StopSignal returns the container's own stop signal configured from
// the image configuration or the default one.
func (c *Container) StopSignal() syscall.Signal {
	if c.stopSignal == "" {
		return defaultStopSignalInt
	}

	s, err := signal.ParseSignal(strings.ToUpper(c.stopSignal))
	if err != nil {
		return defaultStopSignalInt
	}
	return s
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
	if tmpState.InitPid == 0 && tmpState.InitStartTime == "" && tmpState.Pid != 0 {
		if err := tmpState.SetInitPid(tmpState.Pid); err != nil {
			return err
		}
		logrus.Infof("PID information for container %s updated to %d %s", c.ID(), tmpState.InitPid, tmpState.InitStartTime)
	}
	c.state = tmpState
	return nil
}

// SetInitPid initializes the InitPid and InitStartTime for the container state
// given a PID.
// These values should be set once, and not changed again.
func (cstate *ContainerState) SetInitPid(pid int) error {
	if cstate.InitPid != 0 || cstate.InitStartTime != "" {
		return fmt.Errorf("pid and start time already initialized: %d %s", cstate.InitPid, cstate.InitStartTime)
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
	return c.criContainer.Id
}

// CleanupConmonCgroup cleans up conmon's group when using cgroupfs.
func (c *Container) CleanupConmonCgroup() {
	if c.spoofed {
		return
	}
	path := c.ConmonCgroupfsPath()
	if path == "" {
		return
	}
	cg, err := cgroups.Load(path)
	if err != nil {
		logrus.Infof("Error loading conmon cgroup of container %s: %v", c.ID(), err)
		return
	}
	if err := cg.Delete(); err != nil {
		logrus.Infof("Error deleting conmon cgroup of container %s: %v", c.ID(), err)
	}
}

// SetSeccompProfilePath sets the seccomp profile path.
func (c *Container) SetSeccompProfilePath(pp string) {
	c.seccompProfilePath = pp
}

// SeccompProfilePath returns the seccomp profile path.
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
	return c.criContainer.Labels
}

// Annotations returns the annotations of the container.
func (c *Container) Annotations() map[string]string {
	return c.criContainer.Annotations
}

// CrioAnnotations returns the crio annotations of the container.
func (c *Container) CrioAnnotations() map[string]string {
	return c.crioAnnotations
}

// Image returns the image of the container.
func (c *Container) Image() string {
	return c.criContainer.Image.Image
}

// ImageName returns the image name of the container.
func (c *Container) ImageName() string {
	return c.imageName
}

// ImageRef returns the image ref of the container.
func (c *Container) ImageRef() string {
	return c.criContainer.ImageRef
}

// Sandbox returns the sandbox name of the container.
func (c *Container) Sandbox() string {
	return c.criContainer.PodSandboxId
}

// SetSandbox sets the ID of the Sandbox.
func (c *Container) SetSandbox(podSandboxID string) {
	c.criContainer.PodSandboxId = podSandboxID
}

// Dir returns the dir of the container
func (c *Container) Dir() string {
	return c.dir
}

// CheckpointPath returns the path to the directory containing the checkpoint
func (c *Container) CheckpointPath() string {
	// Podman uses 'bundlePath' as base directory for the checkpoint
	// CRI-O uses 'dir' instead of bundlePath as bundlePath seems to be
	// normally based on a tmpfs which does not survive a reboot. Also, as
	// the checkpoint contains all memory pages, it can be as large as the
	// available memory and writing that again to a tmpfs might lead to
	// problems. 'dir' seems to be based on /var
	return filepath.Join(c.dir, metadata.CheckpointDirectory)
}

// Metadata returns the metadata of the container.
func (c *Container) Metadata() *types.ContainerMetadata {
	return c.criContainer.Metadata
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
	return fmt.Sprintf("%s/%s/%s", c.Labels()[kubeletTypes.KubernetesPodNamespaceLabel], c.Labels()[kubeletTypes.KubernetesPodNameLabel], c.Labels()[kubeletTypes.KubernetesContainerNameLabel])
}

// StdinOnce returns whether stdin once is set for the container.
func (c *Container) StdinOnce() bool {
	return c.stdinOnce
}

func (c *Container) exitFilePath() string {
	return filepath.Join(c.dir, "exit")
}

// Living checks if a container's init PID exists and it's running, without calling
// a given runtime directly to check the state, which is expensive.
func (c *Container) Living() error {
	_, _, err := c.pid()
	if err != nil {
		return fmt.Errorf("checking if PID of %s is running failed: %w", c.ID(), err)
	}

	return nil
}

// ProcessState checks if a container's init PID exists and it's running without
// calling a given runtime directly to check the state, which is expensive, and
// additionally returns the current state of the init process as reported by the
// operating system.
func (c *Container) ProcessState() (string, error) {
	_, state, err := c.pid()
	if err != nil {
		return "", fmt.Errorf("checking if PID of %s is running failed: %w", c.ID(), err)
	}

	return state, nil
}

// Pid returns the container's init PID.
// It will fail if the saved PID no longer belongs to the container.
func (c *Container) Pid() (int, error) {
	c.opLock.Lock()
	defer c.opLock.Unlock()

	pid, _, err := c.pid()

	return pid, err
}

// pid returns the container's init PID.
// It checks that we have an InitPid defined in the state, that PID can be found
// and it is the same process that was originally started by the runtime.
func (c *Container) pid() (int, string, error) { //nolint:gocritic // Ignore unnamedResult false positive.
	if c.state == nil {
		return 0, "", ErrNotInitialized
	}
	if c.state.InitPid <= 0 {
		return 0, "", ErrNotInitialized
	}

	// container has stopped (as pid is initialized but the runc state has overwritten it)
	if c.state.Pid == 0 {
		return 0, "", ErrNotFound
	}

	if err := unix.Kill(c.state.InitPid, 0); err != nil {
		if errors.Is(err, unix.ESRCH) {
			return 0, "", ErrNotFound
		}
		return 0, "", fmt.Errorf("error checking if process %d is running: %w", c.state.InitPid, err)
	}

	state, err := c.verifyPid()
	if err != nil {
		return 0, "", err
	}

	// Should the process be terminated or become defunct (zombie), runtimes such as
	// runc and crun will also treat processes as already terminated. As such, CRI-O
	// should do the same, rather than keep requesting a given runtime to kill the
	// container senselessly.
	//
	// Note: Not every platform offers the process state or makes it readily available.
	if state == "X" || state == "Z" {
		return 0, "", ErrNotFound
	}

	return c.state.InitPid, state, nil
}

// verifyPid checks that the start time for the process on the node is the same
// as the start time we saved after creating the container.
// This is the simplest way to verify we are operating on the container
// process, and haven't run into PID wrap.
func (c *Container) verifyPid() (string, error) {
	state, startTime, err := getPidStatData(c.state.InitPid)
	if err != nil {
		return "", err
	}

	if c.state.InitStartTime != startTime {
		return "", fmt.Errorf(
			"PID %d is running but has start time of %s, whereas the saved start time is %s. PID wrap may have occurred",
			c.state.InitPid, startTime, c.state.InitStartTime,
		)
	}

	return state, nil
}

// GetPidStartTimeFromFile reads a file as if it were a /proc/<PID>/stat file,
// looking for a process start time for a given PID. It is abstracted out to
// allow for unit testing.
func GetPidStartTimeFromFile(file string) (string, error) {
	_, startTime, err := getPidStatDataFromFile(file)
	return startTime, err
}

// getPidStartTime returns the process start time for a given PID.
func getPidStartTime(pid int) (string, error) {
	_, startTime, err := getPidStatDataFromFile(fmt.Sprintf(procStatFile, pid))
	return startTime, err
}

// getPidStatData returns the process state and start time for a given PID.
func getPidStatData(pid int) (string, string, error) { //nolint:gocritic // Ignore unnamedResult.
	return getPidStatDataFromFile(fmt.Sprintf(procStatFile, pid))
}

// getPidStatData parses the kernel's /proc/<PID>/stat file,
// looking for the process state and start time for a given PID.
func getPidStatDataFromFile(file string) (string, string, error) { //nolint:gocritic // Ignore unnamedResult.
	f, err := os.Open(file)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, 4096))
	if err != nil {
		return "", "", err
	}

	bracket := bytes.LastIndexByte(data, ')')
	if bracket < 0 {
		return "", "", fmt.Errorf("unable to find ')' in stat file: %w", ErrNotFound)
	}

	// Skip the process name and the white space after the right bracket.
	statFields := bytes.Fields(data[bracket+2:])

	if len(statFields) < startTimeFieldIndex+1 {
		return "", "", fmt.Errorf("unable to parse malformed stat file: %w", ErrNotFound)
	}

	return string(statFields[stateFieldIndex]), string(statFields[startTimeFieldIndex]), nil
}

// ShouldBeStopped checks whether the container state is in a place
// where attempting to stop it makes sense
// a container is not stoppable if it's paused or stopped
// if it's paused, that's an error, and is reported as such
func (c *Container) ShouldBeStopped() error {
	switch c.State().Status {
	case ContainerStateStopped: // no-op
		return ErrContainerStopped
	case ContainerStatePaused:
		return errors.New("cannot stop paused container")
	}
	return nil
}

// Spoofed returns whether this container is spoofed.
// A container should be spoofed when it doesn't have to exist in the container runtime,
// but does need to exist in the storage. The main use of this is when an infra container
// is not needed, but sandbox metadata should be stored with a spoofed infra container.
func (c *Container) Spoofed() bool {
	return c.spoofed
}

// SetAsStopping marks a container as being stopped.
// Returns true if the container was not set as stopping before, and false otherwise (i.e. on subsequent calls)."
func (c *Container) SetAsStopping() (setToStopping bool) {
	c.stopLock.Lock()
	defer c.stopLock.Unlock()
	if !c.stopping {
		c.stopping = true
		return true
	}
	return false
}

func (c *Container) WaitOnStopTimeout(ctx context.Context, timeout int64) {
	c.stopLock.Lock()
	if !c.stopping {
		c.stopLock.Unlock()
		return
	}

	c.stopTimeoutChan <- timeout

	watcher := make(chan struct{}, 1)
	c.stopWatchers = append(c.stopWatchers, watcher)
	c.stopLock.Unlock()

	select {
	case <-ctx.Done():
	case <-watcher:
	}
}

func (c *Container) SetAsDoneStopping() {
	c.stopLock.Lock()
	for _, watcher := range c.stopWatchers {
		close(watcher)
	}
	c.stopWatchers = make([]chan struct{}, 0)
	close(c.stopTimeoutChan)
	c.stopLock.Unlock()
}

func (c *Container) AddManagedPIDNamespace(ns nsmgr.Namespace) {
	c.pidns = ns
}

func (c *Container) RemoveManagedPIDNamespace() error {
	if c.pidns == nil {
		return nil
	}
	return fmt.Errorf("remove PID namespace for container %s: %w", c.ID(), c.pidns.Remove())
}

func (c *Container) IsInfra() bool {
	return c.ID() == c.Sandbox()
}

// nodeLevelPIDNamespace searches through the container spec to see if there is
// a PID namespace specified. If not, it returns `true` (because the runtime spec
// defines a node level namespace as being absent from the Namespaces list)
func (c *Container) nodeLevelPIDNamespace() bool {
	for i := range c.spec.Linux.Namespaces {
		// If it's specified in the namespace list, then it is something other than Node level
		if c.spec.Linux.Namespaces[i].Type == specs.PIDNamespace {
			return false
		}
	}
	return true
}

// Restore returns if the container is marked as being
// restored from a checkpoint
func (c *Container) Restore() bool {
	return c.restore
}

// SetRestore marks the container as being restored from a checkpoint
func (c *Container) SetRestore(restore bool) {
	c.restore = restore
}

func (c *Container) RestoreArchive() string {
	return c.restoreArchive
}

func (c *Container) SetRestoreArchive(restoreArchive string) {
	c.restoreArchive = restoreArchive
}

// AddExecPID registers a PID associated with an exec session.
// It is tracked so exec sessions can be cancelled when the container is being stopped.
// If the PID is conmon, shouldKill should be false, as we should not call SIGKILL on conmon.
// If it is an exec session, shouldKill should be true, as we can't guarantee the exec process
// will have a SIGINT handler.
func (c *Container) AddExecPID(pid int, shouldKill bool) error {
	c.stopLock.Lock()
	defer c.stopLock.Unlock()

	logrus.Debugf("Starting to track exec PID %d for container %s (should kill = %t) ...", pid, c.ID(), shouldKill)
	if c.stopping {
		return errors.New("cannot register an exec PID: container is stopping")
	}
	c.execPIDs[pid] = shouldKill
	return nil
}

// DeleteExecPID is for deregistering a pid after it has exited.
func (c *Container) DeleteExecPID(pid int) {
	c.stopLock.Lock()
	defer c.stopLock.Unlock()
	delete(c.execPIDs, pid)
}

// KillExecPIDs loops through the saved execPIDs and sends a signal to them.
// If shouldKill is true, the signal is SIGKILL. Otherwise, SIGINT.
func (c *Container) KillExecPIDs() {
	c.stopLock.Lock()
	toKill := c.execPIDs
	c.stopLock.Unlock()

	for len(toKill) != 0 {
		unkilled := map[int]bool{}
		for pid, shouldKill := range toKill {
			if pid == 0 {
				// The caller may accidentally register `0` (for instance if the PID of the cmd has already exited)
				// and killing 0 is the way to ask the kernel to kill the whole process group of the calling process.
				// We definitely don't want to kill the CRI-O process group, so add this check just in case.
				continue
			}
			sig := syscall.SIGINT
			if shouldKill {
				sig = syscall.SIGKILL
			}

			logrus.Debugf("Stopping exec PID %d for container %s with signal %s ...", pid, c.ID(), unix.SignalName(sig))
			if err := syscall.Kill(pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
				unkilled[pid] = shouldKill
			}
		}
		toKill = unkilled
		time.Sleep(stopProcessWatchSleep)
	}
}
