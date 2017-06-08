package oci

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/containernetworking/cni/pkg/ns"
	"github.com/docker/docker/pkg/signal"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"k8s.io/apimachinery/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

const (
	defaultStopSignal = "TERM"
)

// Container represents a runtime container.
type Container struct {
	id          string
	name        string
	logPath     string
	labels      fields.Set
	annotations fields.Set
	image       *pb.ImageSpec
	sandbox     string
	netns       ns.NetNS
	terminal    bool
	stdin       bool
	stdinOnce   bool
	privileged  bool
	state       *ContainerState
	metadata    *pb.ContainerMetadata
	opLock      sync.Mutex
	// this is the /var/run/storage/... directory, erased on reboot
	bundlePath string
	// this is the /var/lib/storage/... directory
	dir        string
	stopSignal string
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
func NewContainer(id string, name string, bundlePath string, logPath string, netns ns.NetNS, labels map[string]string, annotations map[string]string, image *pb.ImageSpec, metadata *pb.ContainerMetadata, sandbox string, terminal bool, stdin bool, stdinOnce bool, privileged bool, dir string, created time.Time, stopSignal string) (*Container, error) {
	state := &ContainerState{}
	state.Created = created
	c := &Container{
		id:          id,
		name:        name,
		bundlePath:  bundlePath,
		logPath:     logPath,
		labels:      labels,
		sandbox:     sandbox,
		netns:       netns,
		terminal:    terminal,
		stdin:       stdin,
		stdinOnce:   stdinOnce,
		privileged:  privileged,
		metadata:    metadata,
		annotations: annotations,
		image:       image,
		dir:         dir,
		state:       state,
		stopSignal:  stopSignal,
	}
	return c, nil
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

// Image returns the image of the container.
func (c *Container) Image() *pb.ImageSpec {
	return c.image
}

// Sandbox returns the sandbox name of the container.
func (c *Container) Sandbox() string {
	return c.sandbox
}

// NetNsPath returns the path to the network namespace of the container.
func (c *Container) NetNsPath() (string, error) {
	if c.state == nil {
		return "", fmt.Errorf("container state is not populated")
	}

	if c.netns == nil {
		return fmt.Sprintf("/proc/%d/ns/net", c.state.Pid), nil
	}

	return c.netns.Path(), nil
}

// Metadata returns the metadata of the container.
func (c *Container) Metadata() *pb.ContainerMetadata {
	return c.metadata
}
