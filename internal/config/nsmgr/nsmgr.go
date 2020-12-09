package nsmgr

import (
	"github.com/containers/storage/pkg/idtools"
)

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
