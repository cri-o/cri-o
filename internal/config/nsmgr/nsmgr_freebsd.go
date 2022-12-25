package nsmgr

import (
	"errors"
	"fmt"
	"runtime"
)

// NamespaceManager manages the server's namespaces.
// Specifically, it is an interface for how the server is creating namespaces,
// and can be requested to create namespaces for a pod.
type NamespaceManager struct {
}

// New creates a new NamespaceManager.
func New(namespacesDir, pinnsPath string) *NamespaceManager {
	return &NamespaceManager{}
}

func (mgr *NamespaceManager) Initialize() error {
	return nil
}

// NewPodNamespaces creates new namespaces for a pod. For FreeBSD, there is only
// the vnet network namespace which is implemented as a parent jail for each
// container in the pod.  The caller is responsible for cleaning up the
// namespaces by calling Namespace.Remove().
func (mgr *NamespaceManager) NewPodNamespaces(cfg *PodNamespacesConfig) ([]Namespace, error) {
	if cfg == nil {
		return nil, errors.New("PodNamespacesConfig cannot be nil")
	}
	if len(cfg.Namespaces) == 0 {
		return []Namespace{}, nil
	}

	returnedNamespaces := make([]Namespace, 0, len(cfg.Namespaces))
	for _, cns := range cfg.Namespaces {
		if cns.Type != NETNS {
			return nil, fmt.Errorf("invalid namespace type: %s", cns.Type)
		}
		if cns.Host == false {
			ns, err := GetNamespace(cns.Path, cns.Type)
			if err != nil {
				return nil, err
			}
			returnedNamespaces = append(returnedNamespaces, ns)
		}
	}
	return returnedNamespaces, nil
}

// NamespacePathFromProc returns the namespace path of type nsType for a given pid and type.
func NamespacePathFromProc(nsType NSType, pid int) string {
	return ""
}

func (mgr *NamespaceManager) NamespaceFromProcEntry(pid int, nsType NSType) (_ Namespace, retErr error) {
	return nil, fmt.Errorf("(*NamespaceManager).NamespaceFromProcEntry unsupported on %s", runtime.GOOS)
}
