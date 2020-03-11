package container

import (
	"context"

	"github.com/pkg/errors"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// Container is the main public container interface
type Container interface {
	// SetConfig sets the configuration to the container and validates it
	SetConfig(*pb.ContainerConfig) error

	// Config returns the container configuration
	Config() *pb.ContainerConfig
}

// container is the hidden default type behind the Container interface
type container struct {
	ctx    context.Context
	config *pb.ContainerConfig
}

// New creates a new, empty Sandbox instance
func New(ctx context.Context) Container {
	return &container{
		ctx:    ctx,
		config: nil,
	}
}

// SetConfig sets the configuration to the container and validates it
func (c *container) SetConfig(config *pb.ContainerConfig) error {
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

	c.config = config
	return nil
}

// Config returns the container configuration
func (c *container) Config() *pb.ContainerConfig {
	return c.config
}
