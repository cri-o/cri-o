package oci

import (
	"fmt"
	"sync"
	"time"

	"github.com/containernetworking/cni/pkg/ns"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"k8s.io/apimachinery/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// Container represents a runtime container.
type Container struct {
	id          string
	name        string
	bundlePath  string
	logPath     string
	labels      fields.Set
	annotations fields.Set
	image       *pb.ImageSpec
	sandbox     string
	netns       ns.NetNS
	terminal    bool
	privileged  bool
	state       *ContainerState
	metadata    *pb.ContainerMetadata
	opLock      sync.Mutex
}

// ContainerState represents the status of a container.
type ContainerState struct {
	specs.State
	Created  time.Time `json:"created"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	ExitCode int32     `json:"exitCode"`
}

// NewContainer creates a container object.
func NewContainer(id string, name string, bundlePath string, logPath string, netns ns.NetNS, labels map[string]string, annotations map[string]string, image *pb.ImageSpec, metadata *pb.ContainerMetadata, sandbox string, terminal bool, privileged bool) (*Container, error) {
	c := &Container{
		id:          id,
		name:        name,
		bundlePath:  bundlePath,
		logPath:     logPath,
		labels:      labels,
		sandbox:     sandbox,
		netns:       netns,
		terminal:    terminal,
		privileged:  privileged,
		metadata:    metadata,
		annotations: annotations,
		image:       image,
	}
	return c, nil
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
