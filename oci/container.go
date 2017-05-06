package oci

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/containernetworking/cni/pkg/ns"
	"github.com/docker/docker/pkg/ioutils"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"k8s.io/apimachinery/pkg/fields"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// Container represents a runtime container.
type Container struct {
	Id          string
	Name        string
	BundlePath  string
	LogPath     string
	Labels      fields.Set
	Annotations fields.Set
	Image       *pb.ImageSpec
	Sandbox     string
	netns       ns.NetNS
	Terminal    bool
	Privileged  bool
	State       *ContainerState
	Metadata    *pb.ContainerMetadata
	opLock      sync.Mutex
	StateDir    string
}

// ContainerState represents the status of a container.
type ContainerState struct {
	specs.State
	Created  time.Time `json:"created,omitempty"`
	Started  time.Time `json:"started,omitempty"`
	Finished time.Time `json:"finished,omitempty"`
	ExitCode int32     `json:"exitCode,omitempty"`
}

// NewContainer creates a container object.
func NewContainer(id string, name string, bundlePath string, logPath string, netns ns.NetNS, labels map[string]string, annotations map[string]string, image *pb.ImageSpec, metadata *pb.ContainerMetadata, sandbox string, terminal bool, privileged bool, stateDir string) (*Container, error) {
	c := &Container{
		Id:          id,
		Name:        name,
		BundlePath:  bundlePath,
		LogPath:     logPath,
		Labels:      labels,
		Sandbox:     sandbox,
		netns:       netns,
		Terminal:    terminal,
		Privileged:  privileged,
		Metadata:    metadata,
		Annotations: annotations,
		Image:       image,
		StateDir:    stateDir,
	}
	return c, nil
}

func (c *Container) toDisk() error {
	pth := filepath.Join(c.StateDir, "state")
	jsonSource, err := ioutils.NewAtomicFileWriter(pth, 0644)
	if err != nil {
		return err
	}
	defer jsonSource.Close()
	enc := json.NewEncoder(jsonSource)
	return enc.Encode(c)
}

func (c *Container) FromDisk() error {
	pth := filepath.Join(c.StateDir, "state")

	jsonSource, err := os.Open(pth)
	if err != nil {
		return err
	}
	defer jsonSource.Close()

	dec := json.NewDecoder(jsonSource)

	return dec.Decode(c)
}

// ID returns the id of the container.
func (c *Container) ID() string {
	return c.Id
}

// NetNsPath returns the path to the network namespace of the container.
func (c *Container) NetNsPath() (string, error) {
	if c.State == nil {
		return "", fmt.Errorf("container state is not populated")
	}

	if c.netns == nil {
		return fmt.Sprintf("/proc/%d/ns/net", c.State.Pid), nil
	}

	return c.netns.Path(), nil
}
