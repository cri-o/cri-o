package sandbox

import (
	"context"

	"github.com/cri-o/cri-o/pkg/container"
	"github.com/pkg/errors"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// Sandbox is the interface for managing pod sandboxes
type Sandbox interface {
	Create() error

	Start() error

	Stop() error

	Delete() error

	AddContainer(container.Container) error

	RemoveContainer(container.Container) error

	// SetConfig sets the sandbox configuration and validates it
	SetConfig(*pb.PodSandboxConfig) error

	// Config returns the sandbox configuration
	Config() *pb.PodSandboxConfig
}

// sandbox is the hidden default type behind the Sandbox interface
type sandbox struct {
	ctx    context.Context
	config *pb.PodSandboxConfig
}

// New creates a new, empty Sandbox instance
func New(ctx context.Context) Sandbox {
	return &sandbox{
		ctx:    ctx,
		config: nil,
	}
}

// SetConfig sets the sandbox configuration and validates it
func (s *sandbox) SetConfig(config *pb.PodSandboxConfig) error {
	if s.config != nil {
		return errors.New("config already set")
	}

	if config == nil {
		return errors.New("config is nil")
	}

	if config.GetMetadata() == nil {
		return errors.New("metadata is nil")
	}

	if config.GetMetadata().GetName() == "" {
		return errors.New("PodSandboxConfig.Name should not be empty")
	}
	s.config = config
	return nil
}

// Config returns the sandbox configuration
func (s *sandbox) Config() *pb.PodSandboxConfig {
	return s.config
}
