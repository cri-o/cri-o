package state

import (
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/server/sandbox"
)

// Store stores the state of the CRI-O server, including active pods and containers
type Store interface {
	AddSandbox(s *sandbox.Sandbox) error
	HasSandbox(id string) bool
	DeleteSandbox(id string) error
	// These should modify the associated sandbox without prompting
	AddContainer(c *oci.Container) error
	HasContainer(id, sandboxID string) bool
	DeleteContainer(id, sandboxID string) error
	// These two require full, explicit ID
	GetSandbox(id string) (*sandbox.Sandbox, error)
	GetContainer(id, sandboxID string) (*oci.Container, error)
	// Get ID of sandbox container belongs to
	GetContainerSandbox(id string) (string, error)
	// Following 4 should accept partial names as long as they are globally unique
	LookupSandboxByName(name string) (*sandbox.Sandbox, error)
	LookupSandboxByID(id string) (*sandbox.Sandbox, error)
	LookupContainerByName(name string) (*oci.Container, error)
	LookupContainerByID(id string) (*oci.Container, error)
	GetAllSandboxes() ([]*sandbox.Sandbox, error)
	GetAllContainers() ([]*oci.Container, error)
}
