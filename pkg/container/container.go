package container

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/storage/pkg/stringid"
	"github.com/pkg/errors"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// Container is the main public container interface
type Container interface {
	// All set methods are usually called in order of their definition

	// SetConfig sets the configuration to the container and validates it
	SetConfig(*pb.ContainerConfig, *pb.PodSandboxConfig) error

	// SetNameAndID sets a container name and ID
	SetNameAndID(*pb.PodSandboxMetadata) error

	// Config returns the container configuration
	Config() *pb.ContainerConfig

	SandboxConfig() *pb.PodSandboxConfig

	// ID returns the container ID
	ID() string

	// Name returns the container name
	Name() string

	// SetPrivileged sets the privileged bool for the container
	SetPrivileged() error

	// Privileged returns whether this container is privileged
	Privileged() bool
}

// container is the hidden default type behind the Container interface
type container struct {
	ctx        context.Context
	config     *pb.ContainerConfig
	sboxConfig *pb.PodSandboxConfig
	id         string
	name       string
	privileged bool
}

// New creates a new, empty Sandbox instance
func New(ctx context.Context) Container {
	return &container{
		ctx: ctx,
	}
}

// SetConfig sets the configuration to the container and validates it
func (c *container) SetConfig(config *pb.ContainerConfig, sboxConfig *pb.PodSandboxConfig) error {
	if c.config != nil {
		return errors.New("config already set")
	}

	if config == nil {
		return errors.New("config is nil")
	}

	if config.GetMetadata() == nil {
		return errors.New("metadata is nil")
	}

	if config.GetMetadata().GetName() == "" {
		return errors.New("name is nil")
	}

	if sboxConfig == nil {
		return errors.New("sandbox config is nil")
	}

	if c.sboxConfig != nil {
		return errors.New("sandbox config is already set")
	}

	c.config = config
	c.sboxConfig = sboxConfig
	return nil
}

// SetNameAndID sets a container name and ID
func (c *container) SetNameAndID(sandboxMetadata *pb.PodSandboxMetadata) error {
	if c.config == nil {
		return errors.New("config is not set")
	}

	if sandboxMetadata == nil {
		return errors.New("sandbox metadata is nil")
	}

	id := stringid.GenerateNonCryptoID()
	name := strings.Join([]string{
		"k8s",
		c.config.Metadata.Name,
		sandboxMetadata.Name,
		sandboxMetadata.Namespace,
		sandboxMetadata.Uid,
		fmt.Sprintf("%d", c.config.Metadata.Attempt),
	}, "_")

	c.id = id
	c.name = name
	return nil
}

// Config returns the container configuration
func (c *container) Config() *pb.ContainerConfig {
	return c.config
}

// SandboxConfig returns the sandbox configuration
func (c *container) SandboxConfig() *pb.PodSandboxConfig {
	return c.sboxConfig
}

// ID returns the container ID
func (c *container) ID() string {
	return c.id
}

// Name returns the container name
func (c *container) Name() string {
	return c.name
}

func (c *container) SetPrivileged() error {
	if c.config == nil {
		return nil
	}
	if c.config.GetLinux() == nil {
		return nil
	}
	if c.config.GetLinux().GetSecurityContext() == nil {
		return nil
	}

	if c.sboxConfig == nil {
		return nil
	}

	if c.sboxConfig.GetLinux() == nil {
		return nil
	}

	if c.sboxConfig.GetLinux().GetSecurityContext() == nil {
		return nil
	}

	if c.config.GetLinux().GetSecurityContext().GetPrivileged() {
		if !c.sboxConfig.GetLinux().GetSecurityContext().GetPrivileged() {
			return errors.New("no privileged container allowed in sandbox")
		}
		c.privileged = true
	}
	return nil
}

func (c *container) Privileged() bool {
	return c.privileged
}
