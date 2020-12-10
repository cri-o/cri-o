package nsmgr

import (
	"github.com/containers/storage/pkg/idtools"
)

type NamespaceManager interface {
	NewPodNamespaces(*PodNamespacesConfig) ([]Namespace, error)
}

type PodNamespacesConfig struct {
	Namespaces []*PodNamespaceConfig
	IDMappings *idtools.IDMappings
	Sysctls    map[string]string
}

type PodNamespaceConfig struct {
	Type NSType
	Host bool
	Path string
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
