package sandbox

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/storage/pkg/stringid"
	"github.com/cri-o/cri-o/pkg/container"
	"github.com/cri-o/cri-o/server/cri/types"
	"github.com/pkg/errors"
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
	SetConfig(*types.PodSandboxConfig) error

	// SetNameAndID sets the sandbox name and ID
	SetNameAndID() error

	// Config returns the sandbox configuration
	Config() *types.PodSandboxConfig

	// ID returns the id of the pod sandbox
	ID() string

	// Name returns the id of the pod sandbox
	Name() string
}

// sandbox is the hidden default type behind the Sandbox interface
type sandbox struct {
	ctx    context.Context
	config *types.PodSandboxConfig
	id     string
	name   string
}

// New creates a new, empty Sandbox instance
func New(ctx context.Context) Sandbox {
	return &sandbox{
		ctx:    ctx,
		config: nil,
	}
}

// SetConfig sets the sandbox configuration and validates it
func (s *sandbox) SetConfig(config *types.PodSandboxConfig) error {
	if s.config != nil {
		return errors.New("config already set")
	}

	if config == nil {
		return errors.New("config is nil")
	}

	if config.Metadata == nil {
		return errors.New("metadata is nil")
	}

	if config.Metadata.Name == "" {
		return errors.New("PodSandboxConfig.Metadata.Name should not be empty")
	}
	s.config = config
	return nil
}

// SetNameAndID sets the sandbox name and ID
func (s *sandbox) SetNameAndID() error {
	if s.config == nil {
		return errors.New("config is nil")
	}

	if s.config.Metadata.Namespace == "" {
		return errors.New("cannot generate pod name without namespace")
	}

	if s.config.Metadata.Name == "" {
		return errors.New("cannot generate pod name without name in metadata")
	}

	s.id = stringid.GenerateNonCryptoID()
	s.name = strings.Join([]string{
		"k8s",
		s.config.Metadata.Name,
		s.config.Metadata.Namespace,
		s.config.Metadata.UID,
		fmt.Sprintf("%d", s.config.Metadata.Attempt),
	}, "_")

	return nil
}

// Config returns the sandbox configuration
func (s *sandbox) Config() *types.PodSandboxConfig {
	return s.config
}

// ID returns the id of the pod sandbox
func (s *sandbox) ID() string {
	return s.id
}

// Name returns the id of the pod sandbox
func (s *sandbox) Name() string {
	return s.name
}
