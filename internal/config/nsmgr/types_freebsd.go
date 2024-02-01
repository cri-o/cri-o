//go:build freebsd
// +build freebsd

package nsmgr

import (
	"sync"
)

// supportedNamespacesForPinning returns a slice of
// the names of namespaces that CRI-O supports
// pinning.
func supportedNamespacesForPinning() []NSType {
	return []NSType{NETNS}
}

type PodNamespacesConfig struct {
	Namespaces []*PodNamespaceConfig
}

type PodNamespaceConfig struct {
	Type NSType
	Host bool
	Path string
}

// namespace is the internal implementation of the Namespace interface.
type namespace struct {
	sync.Mutex
	closed   bool
	nsType   NSType
	jailName string
}

// Path returns the bind mount path of the namespace.
func (n *namespace) Path() string {
	if n == nil {
		return ""
	}
	return n.jailName
}

// Type returns the namespace type (net, ipc, user, pid or uts).
func (n *namespace) Type() NSType {
	return n.nsType
}

// Remove ensures this namespace is closed and removed.
func (n *namespace) Remove() error {
	n.Lock()
	defer n.Unlock()

	if n.closed {
		// Remove() can be called multiple
		// times without returning an error.
		return nil
	}

	n.closed = true
	return nil
}

// GetNamespace takes a path and type, checks if it is a namespace, and if so
// returns an instance of the Namespace interface.
func GetNamespace(jailName string, nsType NSType) (Namespace, error) {
	return &namespace{nsType: nsType, jailName: jailName}, nil
}
