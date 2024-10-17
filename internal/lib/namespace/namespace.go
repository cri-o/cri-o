package namespace

import "github.com/cri-o/cri-o/internal/config/nsmgr"

// ManagedNamespace is a structure that holds all the necessary information a caller would
// need for a sandbox managed namespace
// Where nsmgr.Namespace does hold similar information, ManagedNamespace exists to allow this library
// to not return data not necessarily in a Namespace (for instance, when a namespace is not managed
// by CRI-O, but instead is based off of the infra pid).
type ManagedNamespace struct {
	nsPath string
	nsType nsmgr.NSType
}

// Type returns the namespace type.
func (m *ManagedNamespace) Type() nsmgr.NSType {
	return m.nsType
}

// Type returns the namespace path.
func (m *ManagedNamespace) Path() string {
	return m.nsPath
}

func NewManagedNamespace(nsPath string, nsType nsmgr.NSType) *ManagedNamespace {
	return &ManagedNamespace{
		nsPath: nsPath,
		nsType: nsType,
	}
}
