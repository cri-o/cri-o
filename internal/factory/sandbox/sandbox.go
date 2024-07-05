package sandbox

import (
	"errors"
	"strconv"
	"strings"

	"github.com/containers/storage/pkg/idtools"
	"github.com/containers/storage/pkg/stringid"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/storage"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

// Sandbox is the interface for managing pod sandboxes.
type Sandbox interface {
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

	// InitInfraContainer initializes the sandbox's infra container
	InitInfraContainer(*libconfig.Config, *storage.ContainerInfo, *idtools.IDMappings) error

	// Spec returns the infra container's generator
	// Must be called after InitInfraContainer
	Spec() *generate.Generator

	// ResolvPath returns the sandbox's resolvPath
	ResolvPath() string
}

// sandbox is the hidden default type behind the Sandbox interface.
type sandbox struct {
	config     *types.PodSandboxConfig
	id         string
	name       string
	infra      container.Container
	resolvPath string
}

// New creates a new, empty Sandbox instance.
func New() Sandbox {
	return &sandbox{
		config: nil,
	}
}

// SetConfig sets the sandbox configuration and validates it.
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
		return errors.New("metadata.Name should not be empty")
	}

	if config.Linux == nil {
		config.Linux = &types.LinuxPodSandboxConfig{}
	}

	if config.Linux.SecurityContext == nil {
		config.Linux.SecurityContext = &types.LinuxSandboxSecurityContext{
			NamespaceOptions: &types.NamespaceOption{},
			SelinuxOptions:   &types.SELinuxOption{},
			RunAsUser:        &types.Int64Value{},
			RunAsGroup:       &types.Int64Value{},
			Seccomp:          &types.SecurityProfile{},
			Apparmor:         &types.SecurityProfile{},
		}
	}

	s.config = config
	return nil
}

// SetNameAndID sets the sandbox name and ID.
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

	if s.config.Metadata.Uid == "" {
		return errors.New("cannot generate pod name without uid in metadata")
	}

	s.id = stringid.GenerateNonCryptoID()
	s.name = strings.Join([]string{
		"k8s",
		s.config.Metadata.Name,
		s.config.Metadata.Namespace,
		s.config.Metadata.Uid,
		strconv.FormatUint(uint64(s.config.Metadata.Attempt), 10),
	}, "_")

	return nil
}

// Config returns the sandbox configuration.
func (s *sandbox) Config() *types.PodSandboxConfig {
	return s.config
}

// ID returns the id of the pod sandbox.
func (s *sandbox) ID() string {
	return s.id
}

// Name returns the id of the pod sandbox.
func (s *sandbox) Name() string {
	return s.name
}

func (s *sandbox) ResolvPath() string {
	return s.resolvPath
}
