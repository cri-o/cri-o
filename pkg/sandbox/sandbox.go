package sandbox

import (
	"context"

	"github.com/cri-o/cri-o/pkg/container"
)

// Sandbox is the interface for managing pod sandboxes
type Sandbox interface {
	Create() error

	Start() error

	Stop() error

	Delete() error

	AddContainer(container.Container) error

	RemoveContainer(container.Container) error
}

// sandbox is the hidden default type behind the Sandbox interface
type sandbox struct {
	ctx context.Context
}

// New creates a new, empty Sandbox instance
func New(ctx context.Context) Sandbox {
	return &sandbox{ctx}
}
