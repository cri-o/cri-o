package server

import (
	"github.com/kubernetes-incubator/cri-o/oci"
)

// StateStore stores the state of the CRI-O server, including active pods and
// containers
type StateStore interface {
	AddSandbox(s *sandbox) error
	HasSandbox(id string) bool
	DeleteSandbox(id string) error
	// These should modify the associated sandbox without prompting
	AddContainer(c *oci.Container, sandboxID string) error
	HasContainer(id, sandboxID string) bool
	DeleteContainer(id, sandboxID string) error
	// These two require full, explicit ID
	GetSandbox(id string) (*sandbox, error)
	GetContainer(id, sandboxID string) (*oci.Container, error)
	// Get ID of sandbox container belongs to
	GetContainerSandbox(id string) (string, error)
	// Following 4 should accept partial names as long as they are globally unique
	LookupSandboxByName(name string) (*sandbox, error)
	LookupSandboxByID(id string) (*sandbox, error)
	LookupContainerByName(name string) (*oci.Container, error)
	LookupContainerByID(id string) (*oci.Container, error)
	GetAllSandboxes() ([]*sandbox, error)
	GetAllContainers() ([]*oci.Container, error)
}
