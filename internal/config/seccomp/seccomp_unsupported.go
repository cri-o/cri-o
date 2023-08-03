//go:build !(linux && cgo)
// +build !linux !cgo

package seccomp

import (
	"context"

	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Config is the global seccomp configuration type
type Config struct {
	enabled bool
}

// Notifier wraps a seccomp notifier instance for a container.
type Notifier struct {
}

// Notification is a seccomp notification which gets sent to the CRI-O server.
type Notification struct {
}

// New creates a new default seccomp configuration instance
func New() *Config {
	return &Config{
		enabled: false,
	}
}

// Setup can be used to setup the seccomp profile.
func (c *Config) Setup(
	ctx context.Context,
	msgChan chan Notification,
	containerID string,
	annotations map[string]string,
	specGenerator *generate.Generator,
	profileField *types.SecurityProfile,
) (*Notifier, string, error) {
	return nil, "", nil
}

// SetUseDefaultWhenEmpty uses the default seccomp profile if true is passed as
// argument, otherwise unconfined.
func (c *Config) SetUseDefaultWhenEmpty(to bool) {
}

// Returns whether the seccomp config is set to
// use default profile when the profile is empty
func (c *Config) UseDefaultWhenEmpty() bool {
	return false
}

// SetNotifierPath sets the default path for creating seccomp notifier sockets.
func (c *Config) SetNotifierPath(path string) {
}

// NotifierPath returns the currently used seccomp notifier base path.
func (c *Config) NotifierPath() string {
	return ""
}

// LoadProfile can be used to load a seccomp profile from the provided path.
// This method will not fail if seccomp is disabled.
func (c *Config) LoadProfile(profilePath string) error {
	return nil
}

// LoadDefaultProfile sets the internal default profile.
func (c *Config) LoadDefaultProfile() error {
	return nil
}

// NewNotifier starts the notifier for the provided arguments.
func NewNotifier(
	ctx context.Context,
	msgChan chan Notification,
	containerID, listenerPath string,
	annotationMap map[string]string,
) (*Notifier, error) {
	return nil, nil
}

// Close can be used to close the notifier listener.
func (*Notifier) Close() error {
	return nil
}

func (*Notifier) AddSyscall(syscall string) {
}

func (*Notifier) UsedSyscalls() string {
	return ""
}

func (*Notifier) StopContainers() bool {
	return false
}

func (*Notifier) OnExpired(callback func()) {
}

func (*Notification) Ctx() context.Context {
	return nil
}

func (*Notification) ContainerID() string {
	return ""
}

func (*Notification) Syscall() string {
	return ""
}
