package nsmgr

import (
	"github.com/containers/storage/pkg/idtools"
)

// NamespaceManager manages the server's namespaces.
// Specifically, it is an interface for how the server is creating namespaces (managing or not),
// and can be requested to create namespaces for a pod.
type NamespaceManager interface {
	NewPodNamespaces(managedNamespaces []NSType, idMappings *idtools.IDMappings, sysctls map[string]string) ([]Namespace, error)
}

func New(namespacesDir, pinnsPath string) NamespaceManager {
	return &managedNamespaceManager{
		namespacesDir: namespacesDir,
		pinnsPath:     pinnsPath,
	}
}

type managedNamespaceManager struct {
	namespacesDir string
	pinnsPath     string
}
